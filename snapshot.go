package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"
)

// SnapshotStore persists daily energy_total_wh readings to a JSON file.
// Used to compute calendar-period energy metrics (today, month, year).
type SnapshotStore struct {
	path   string
	tz     *time.Location
	logger *slog.Logger

	mu   sync.Mutex
	data map[string]float64 // date string -> energy_total_wh

	lastKnownGood float64 // for counter reset detection
}

// NewSnapshotStore loads or creates a snapshot file.
func NewSnapshotStore(path string, tz *time.Location, logger *slog.Logger) (*SnapshotStore, error) {
	s := &SnapshotStore{
		path:   path,
		tz:     tz,
		logger: logger,
	}

	data, err := loadSnapshotFile(path, logger)
	if err != nil {
		return nil, err
	}
	s.data = data

	// Set lastKnownGood from the most recent snapshot
	if latest, ok := s.latestValue(); ok {
		s.lastKnownGood = latest
	}

	logger.Info("snapshot store loaded", "path", path, "entries", len(s.data), "timezone", tz.String())
	return s, nil
}

func loadSnapshotFile(path string, logger *slog.Logger) (map[string]float64, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]float64), nil
	}
	if err != nil {
		return nil, err
	}

	// Clean up stale temp file from interrupted writes
	os.Remove(path + ".tmp")

	var data map[string]float64
	if err := json.Unmarshal(b, &data); err != nil {
		logger.Warn("corrupt snapshot file, starting fresh", "path", path, "error", err)
		return make(map[string]float64), nil
	}
	return data, nil
}

// Record saves a snapshot for today if one doesn't exist yet.
// Returns false if the value was rejected (invalid or counter reset).
func (s *SnapshotStore) Record(energyTotal float64) bool {
	if energyTotal <= 0 {
		return false
	}

	s.mu.Lock()

	// Counter reset detection
	if s.lastKnownGood > 0 && energyTotal < s.lastKnownGood*0.9 {
		s.logger.Warn("energy counter reset detected, re-baselining",
			"previous", s.lastKnownGood, "current", energyTotal)
		// Reset all snapshots — historical deltas are now meaningless
		s.data = make(map[string]float64)
	}
	s.lastKnownGood = energyTotal

	today := time.Now().In(s.tz).Format("2006-01-02")
	if _, exists := s.data[today]; exists {
		s.mu.Unlock()
		return true
	}

	s.data[today] = energyTotal

	// Prune old entries
	s.pruneOldEntries()

	// Copy for serialization outside lock
	snapshot := make(map[string]float64, len(s.data))
	for k, v := range s.data {
		snapshot[k] = v
	}
	s.mu.Unlock()

	if err := atomicWriteJSON(s.path, snapshot); err != nil {
		s.logger.Error("failed to write snapshot file", "error", err)
		return false
	}

	s.logger.Info("daily snapshot recorded", "date", today, "energy_total_wh", energyTotal)
	return true
}

// EnergyToday returns energy produced since today's snapshot.
func (s *SnapshotStore) EnergyToday(current float64) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().In(s.tz).Format("2006-01-02")
	baseline, ok := s.data[today]
	if !ok {
		return 0, false
	}
	delta := current - baseline
	if delta < 0 {
		return 0, true
	}
	return delta, true
}

// EnergyMonth returns energy produced since the 1st of the current month.
func (s *SnapshotStore) EnergyMonth(current float64) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().In(s.tz)
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.tz).Format("2006-01-02")

	baseline, ok := s.findClosestAfter(firstOfMonth)
	if !ok {
		return 0, false
	}
	delta := current - baseline
	if delta < 0 {
		return 0, true
	}
	return delta, true
}

// EnergyYear returns energy produced since Jan 1st of the current year.
func (s *SnapshotStore) EnergyYear(current float64) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().In(s.tz)
	jan1 := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, s.tz).Format("2006-01-02")

	baseline, ok := s.findClosestAfter(jan1)
	if !ok {
		return 0, false
	}
	delta := current - baseline
	if delta < 0 {
		return 0, true
	}
	return delta, true
}

// SnapshotAge returns how long ago the most recent snapshot was recorded.
func (s *SnapshotStore) SnapshotAge() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.data) == 0 {
		return time.Duration(0)
	}

	var latest string
	for k := range s.data {
		if k > latest {
			latest = k
		}
	}

	t, err := time.ParseInLocation("2006-01-02", latest, s.tz)
	if err != nil {
		return time.Duration(0)
	}
	return time.Since(t)
}

// findClosestAfter returns the value of the earliest snapshot on or after the given date.
// Must be called with s.mu held.
func (s *SnapshotStore) findClosestAfter(date string) (float64, bool) {
	// Exact match first
	if v, ok := s.data[date]; ok {
		return v, true
	}

	// Find earliest date >= target
	var best string
	for k := range s.data {
		if k >= date && (best == "" || k < best) {
			best = k
		}
	}
	if best == "" {
		return 0, false
	}
	return s.data[best], true
}

// latestValue returns the most recent snapshot value.
func (s *SnapshotStore) latestValue() (float64, bool) {
	if len(s.data) == 0 {
		return 0, false
	}
	var latest string
	for k := range s.data {
		if k > latest {
			latest = k
		}
	}
	return s.data[latest], true
}

// pruneOldEntries removes daily entries older than 90 days,
// but keeps the 1st of each month and Jan 1st entries.
// Must be called with s.mu held.
func (s *SnapshotStore) pruneOldEntries() {
	cutoff := time.Now().In(s.tz).AddDate(0, 0, -90).Format("2006-01-02")

	for k := range s.data {
		if k >= cutoff {
			continue
		}
		// Keep 1st of month and Jan 1st
		if len(k) == 10 && (k[8:] == "01") {
			continue
		}
		delete(s.data, k)
	}
}

func atomicWriteJSON(path string, data map[string]float64) error {
	// Sort keys for deterministic, human-readable output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make([]struct {
		K string
		V float64
	}, len(keys))
	for i, k := range keys {
		ordered[i].K = k
		ordered[i].V = data[k]
	}

	// Build JSON manually for sorted output
	out := make(map[string]float64, len(data))
	for _, kv := range ordered {
		out[kv.K] = kv.V
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}

	// fsync to ensure data is on disk before rename
	f, err := os.Open(tmp)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()

	return os.Rename(tmp, path)
}
