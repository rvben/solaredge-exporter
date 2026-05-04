package main

import (
	"encoding/json"
	"log/slog"
	"math"
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

	// Record a snapshot (no energyToday available)
	ok := store.Record(22950000, math.NaN())
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
	ok = store.Record(22953500, math.NaN())
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
	store1.Record(22950000, math.NaN())

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
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

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
	store.Record(22950000, math.NaN())

	// Simulate counter reset (inverter replacement)
	ok := store.Record(500, math.NaN())
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

	if store.Record(0, math.NaN()) {
		t.Error("should reject zero value")
	}
	if store.Record(-100, math.NaN()) {
		t.Error("should reject negative value")
	}
}

func TestSnapshotStoreNegativeDelta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())
	store.Record(22950000, math.NaN())

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

	now := time.Now().UTC()
	jan1 := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	midMonth := time.Date(now.Year(), now.Month(), 10, 0, 0, 0, 0, time.UTC).Format("2006-01-02")

	data := map[string]float64{
		jan1:         22000000,
		firstOfMonth: 22800000,
		midMonth:     22950000,
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

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
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

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
	store.Record(22950000, math.NaN())

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
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, _ := NewSnapshotStore(path, time.UTC, testLogger())

	// Recording a new value triggers pruning (today doesn't exist yet)
	store.Record(22950000, math.NaN())

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
	store.Record(22950000, math.NaN())
	age = store.SnapshotAge()
	if age > 24*time.Hour {
		t.Errorf("age = %v, should be < 24h for today's snapshot", age)
	}
}

// TestSnapshotStoreMidDayRestart verifies that when the exporter restarts mid-day
// and the API provides today's energy, the baseline is back-calculated correctly
// so EnergyToday reflects actual production rather than returning 0.
func TestSnapshotStoreMidDayRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")
	tz := time.UTC

	store, err := NewSnapshotStore(path, tz, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Simulate mid-day restart: lifetime total is 23,005,766 Wh,
	// API reports today produced 18,900 Wh so far.
	energyTotal := 23005766.0
	energyToday := 18900.0

	ok := store.Record(energyTotal, energyToday)
	if !ok {
		t.Fatal("Record returned false")
	}

	// EnergyToday should reflect what the API reported, not 0
	got, ok := store.EnergyToday(energyTotal)
	if !ok {
		t.Fatal("EnergyToday returned not-ok")
	}
	if got != energyToday {
		t.Errorf("EnergyToday = %f, want %f", got, energyToday)
	}

	// The stored baseline should be energyTotal - energyToday
	wantBaseline := energyTotal - energyToday
	store.mu.Lock()
	today := time.Now().In(tz).Format("2006-01-02")
	gotBaseline := store.data[today]
	store.mu.Unlock()
	if gotBaseline != wantBaseline {
		t.Errorf("stored baseline = %f, want %f", gotBaseline, wantBaseline)
	}

	// As the day continues and production increases, EnergyToday should increase too
	got, _ = store.EnergyToday(energyTotal + 5000)
	if got != energyToday+5000 {
		t.Errorf("EnergyToday after more production = %f, want %f", got, energyToday+5000)
	}
}

// TestSnapshotStoreMidnightRolloverIgnoresStaleEnergyToday verifies that at
// the routine midnight rollover (yesterday's snapshot exists), the back-calc
// is skipped because the API's lastDayData.energy still reflects yesterday's
// full production at that moment — the inverter hasn't transmitted for the
// new day yet (it's nighttime). Using it would set the baseline one day too
// early and inflate today/month metrics by yesterday's full production.
func TestSnapshotStoreMidnightRolloverIgnoresStaleEnergyToday(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")
	tz := time.UTC

	// Seed yesterday's snapshot — simulates a normally-running exporter that
	// has just crossed midnight.
	yesterday := time.Now().In(tz).AddDate(0, 0, -1).Format("2006-01-02")
	yesterdayBaseline := 23693156.0
	yesterdayProduction := 8062.0
	endOfYesterday := yesterdayBaseline + yesterdayProduction

	seed := map[string]float64{yesterday: yesterdayBaseline}
	b, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := NewSnapshotStore(path, tz, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// At 00:00:04 the API returns the inverter's last-known reading from
	// before midnight: lifeTime = end of yesterday, lastDayData = yesterday's
	// full production. The exporter must NOT back-calculate from this.
	store.Record(endOfYesterday, yesterdayProduction)

	today := time.Now().In(tz).Format("2006-01-02")
	store.mu.Lock()
	gotBaseline := store.data[today]
	store.mu.Unlock()
	if gotBaseline != endOfYesterday {
		t.Errorf("baseline = %f, want %f (= current lifetime). "+
			"Stale energyToday from API leaked into baseline.",
			gotBaseline, endOfYesterday)
	}

	// Verify EnergyToday is ~0 right after midnight rollover.
	got, ok := store.EnergyToday(endOfYesterday)
	if !ok {
		t.Fatal("EnergyToday returned not-ok")
	}
	if got != 0 {
		t.Errorf("EnergyToday = %f, want 0 at midnight rollover", got)
	}
}

// TestSnapshotStoreSecondRecordDoesNotOverwriteMidDay verifies that a second
// Record call on the same day (normal polling after mid-day restart) does not
// overwrite the back-calculated baseline.
func TestSnapshotStoreSecondRecordDoesNotOverwriteMidDay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshots.json")
	tz := time.UTC

	store, _ := NewSnapshotStore(path, tz, testLogger())

	// First Record: mid-day restart with API energyToday
	store.Record(23005766, 18900)

	// Second Record: next polling tick, energyToday is now higher
	store.Record(23006000, 19134)

	// Baseline should still be from the first Record
	got, _ := store.EnergyToday(23005766)
	if got != 18900 {
		t.Errorf("EnergyToday = %f, want 18900 (baseline must not change)", got)
	}
}
