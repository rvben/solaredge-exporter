package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestParseAPIResponse(t *testing.T) {
	raw := `{
		"overview": {
			"lastUpdateTime": "2026-03-23 15:00:00",
			"currentPower": {"power": 2645.0},
			"lastDayData": {"energy": 12500.0},
			"lastMonthData": {"energy": 350000.0},
			"lifeTimeData": {"energy": 22948250.0}
		}
	}`

	var resp apiOverviewResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	data := resp.toInverterData()
	if data.ACPower != 2645.0 {
		t.Errorf("ACPower = %f, want 2645.0", data.ACPower)
	}
	if data.EnergyTotal != 22948250.0 {
		t.Errorf("EnergyTotal = %f, want 22948250.0", data.EnergyTotal)
	}
	if !data.Reachable {
		t.Error("expected Reachable = true")
	}
}

func TestAPIBackendPolling(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"overview": map[string]any{
				"lastUpdateTime": "2026-03-23 15:00:00",
				"currentPower":   map[string]any{"power": 1000.0},
				"lifeTimeData":   map[string]any{"energy": 50000.0},
			},
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend, err := NewAPIBackend(ctx, server.URL, "fake-key", "12345", 100*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("NewAPIBackend failed: %v", err)
	}
	defer backend.Close()

	data, err := backend.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if data.ACPower != 1000.0 {
		t.Errorf("ACPower = %f, want 1000.0", data.ACPower)
	}

	// Wait for at least one more poll cycle
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	count := callCount
	mu.Unlock()
	if count < 2 {
		t.Errorf("expected at least 2 API calls, got %d", count)
	}
}

func TestAPIBackend429Handling(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"overview": map[string]any{
				"currentPower": map[string]any{"power": 1000.0},
				"lifeTimeData": map[string]any{"energy": 50000.0},
			},
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend, err := NewAPIBackend(ctx, server.URL, "fake-key", "12345", 50*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("NewAPIBackend failed: %v", err)
	}
	defer backend.Close()

	// After 429, Read() should still return cached data
	time.Sleep(150 * time.Millisecond)
	data, err := backend.Read()
	if err != nil {
		t.Fatalf("Read after 429 failed: %v", err)
	}
	if data.ACPower != 1000.0 {
		t.Errorf("expected cached ACPower = 1000.0, got %f", data.ACPower)
	}
}

func TestAPIBackendConsecutive429sDoNotAccumulate(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	callTimes := []time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		callTimes = append(callTimes, time.Now())
		count := callCount
		mu.Unlock()
		if count == 1 {
			// First call succeeds (startup)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"overview": map[string]any{
					"currentPower": map[string]any{"power": 1000.0},
					"lifeTimeData": map[string]any{"energy": 50000.0},
				},
			})
			return
		}
		// All subsequent calls return 429
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interval := 50 * time.Millisecond
	backend, err := NewAPIBackend(ctx, server.URL, "fake-key", "12345", interval, nil)
	if err != nil {
		t.Fatalf("NewAPIBackend failed: %v", err)
	}
	defer backend.Close()

	// Wait for at least 3 poll cycles (2 consecutive 429s)
	time.Sleep(400 * time.Millisecond)

	// Verify second 429 used 2x interval, not 4x
	// Gap between call 2 and 3 should be ~100ms (2x50ms), not ~200ms (4x50ms)
	mu.Lock()
	times := make([]time.Time, len(callTimes))
	copy(times, callTimes)
	mu.Unlock()

	if len(times) >= 3 {
		gap := times[2].Sub(times[1])
		// Allow generous tolerance, but it should be closer to 100ms than 200ms
		if gap > 180*time.Millisecond {
			t.Errorf("consecutive 429s accumulated: gap between 2nd and 3rd call was %v, expected ~%v", gap, interval*2)
		}
	}
}
