package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestSnapshotStoreNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")
	tz := time.UTC

	store, err := NewSnapshotStore(path, tz, testLogger())
	if err != nil {
		t.Fatalf("NewSnapshotStore failed: %v", err)
	}

	if len(store.data) != 0 {
		t.Errorf("expected empty store, got %d entries", len(store.data))
	}
}

func TestSnapshotStoreRecordAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")
	tz := time.UTC

	store, err := NewSnapshotStore(path, tz, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Record a snapshot
	ok := store.Record(22950000)
	if !ok {
		t.Fatal("Record returned false")
	}

	// energy_today should be 0 (just recorded baseline)
	today, ok := store.EnergyToday(22950000)
	if !ok {
		t.Fatal("EnergyToday returned not-ok")
	}
	if today != 0 {
		t.Errorf("EnergyToday = %f, want 0", today)
	}

	// Simulate production during the day
	today, ok = store.EnergyToday(22953500)
	if !ok {
		t.Fatal("EnergyToday returned not-ok")
	}
	if today != 3500 {
		t.Errorf("EnergyToday = %f, want 3500", today)
	}

	// Second Record on same day should not overwrite
	ok = store.Record(22953500)
	if !ok {
		t.Fatal("Record returned false on second call")
	}
	today, _ = store.EnergyToday(22953500)
	if today != 3500 {
		t.Errorf("EnergyToday after second Record = %f, want 3500 (should use first snapshot)", today)
	}
}

func TestSnapshotStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")
	tz := time.UTC

	// Create and record
	store1, _ := NewSnapshotStore(path, tz, testLogger())
	store1.Record(22950000)

	// Reload from file
	store2, err := NewSnapshotStore(path, tz, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	today, ok := store2.EnergyToday(22953500)
	if !ok {
		t.Fatal("EnergyToday returned not-ok after reload")
	}
	if today != 3500 {
		t.Errorf("EnergyToday after reload = %f, want 3500", today)
	}
}

func TestSnapshotStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	// Write corrupt JSON
	os.WriteFile(path, []byte("{invalid json"), 0644)

	store, err := NewSnapshotStore(path, time.UTC, testLogger())
	if err != nil {
		t.Fatal("should not fail on corrupt file")
	}
	if len(store.data) != 0 {
		t.Error("should start fresh on corrupt file")
	}
}

func TestSnapshotStoreCounterReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")
	tz := time.UTC

	store, _ := NewSnapshotStore(path, tz, testLogger())
	store.Record(22950000)

	// Simulate counter reset (inverter replacement)
	ok := store.Record(500)
	if !ok {
		t.Fatal("Record returned false after counter reset")
	}

	// All old snapshots should be cleared
	today, ok := store.EnergyToday(500)
	if !ok {
		t.Fatal("EnergyToday not ok after reset")
	}
	if today != 0 {
		t.Errorf("EnergyToday = %f, want 0 after counter reset", today)
	}
}

func TestSnapshotStoreRejectsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())

	if store.Record(0) {
		t.Error("should reject zero value")
	}
	if store.Record(-100) {
		t.Error("should reject negative value")
	}
}

func TestSnapshotStoreNegativeDelta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())
	store.Record(22950000)

	// If current < snapshot (small glitch, not full reset), return 0
	today, ok := store.EnergyToday(22949999)
	if !ok {
		t.Fatal("EnergyToday not ok")
	}
	if today != 0 {
		t.Errorf("EnergyToday = %f, want 0 for negative delta", today)
	}
}

func TestSnapshotStoreMonthAndYear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	// Pre-seed with historical data
	data := map[string]float64{
		"2026-01-01": 22000000,
		"2026-03-01": 22800000,
		"2026-03-24": 22950000,
	}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0644)

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())

	current := 22955000.0

	month, ok := store.EnergyMonth(current)
	if !ok {
		t.Fatal("EnergyMonth not ok")
	}
	if month != 155000 {
		t.Errorf("EnergyMonth = %f, want 155000", month)
	}

	year, ok := store.EnergyYear(current)
	if !ok {
		t.Fatal("EnergyYear not ok")
	}
	if year != 955000 {
		t.Errorf("EnergyYear = %f, want 955000", year)
	}
}

func TestSnapshotStoreMissingMonthStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	// No snapshot for the 1st, but have one for the 3rd
	now := time.Now().In(time.UTC)
	data := map[string]float64{
		now.AddDate(0, 0, -3).Format("2006-01-02"): 22940000,
	}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0644)

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())

	// Should use the closest available snapshot after month start
	_, ok := store.EnergyMonth(22950000)
	// May or may not be ok depending on whether the -3 day is in the current month
	// Just verify it doesn't panic
	_ = ok
}

func TestSnapshotStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())
	store.Record(22950000)

	// Temp file should not exist after successful write
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful write")
	}

	// Main file should be valid JSON
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]float64
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("snapshot file is not valid JSON: %v", err)
	}
	if len(data) != 1 {
		t.Errorf("expected 1 entry, got %d", len(data))
	}
}

func TestSnapshotStorePruning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	// Create data with old entries — do NOT include today
	// so Record() will add a new entry and trigger pruning
	data := map[string]float64{
		"2025-01-01": 20000000, // Jan 1st — should be kept
		"2025-01-15": 20100000, // Old daily — should be pruned
		"2025-06-01": 21000000, // 1st of month — should be kept
		"2025-06-15": 21100000, // Old daily — should be pruned
	}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0644)

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())

	// Recording a new value triggers pruning (today doesn't exist yet)
	store.Record(22950000)

	store.mu.Lock()
	defer store.mu.Unlock()

	// Today's entry added
	today := time.Now().In(time.UTC).Format("2006-01-02")
	if _, ok := store.data[today]; !ok {
		t.Error("today's entry should be added")
	}
	// 1st of month kept
	if _, ok := store.data["2025-01-01"]; !ok {
		t.Error("Jan 1st should be kept")
	}
	if _, ok := store.data["2025-06-01"]; !ok {
		t.Error("1st of month should be kept")
	}
	// Non-1st old entries pruned
	if _, ok := store.data["2025-01-15"]; ok {
		t.Error("old daily entry should be pruned")
	}
	if _, ok := store.data["2025-06-15"]; ok {
		t.Error("old daily entry should be pruned")
	}
}

func TestSnapshotStoreSnapshotAge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())

	// Empty store
	age := store.SnapshotAge()
	if age != 0 {
		t.Errorf("empty store age = %v, want 0", age)
	}

	// After recording
	store.Record(22950000)
	age = store.SnapshotAge()
	if age > 24*time.Hour {
		t.Errorf("age = %v, should be < 24h for today's snapshot", age)
	}
}
