package main

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// controllableBackend is a mock Backend whose behavior can be changed mid-test.
type controllableBackend struct {
	data *InverterData
	err  error
}

func (b *controllableBackend) Read() (*InverterData, error) {
	if b.err != nil {
		return nil, b.err
	}
	cp := *b.data
	return &cp, nil
}

func (b *controllableBackend) Close() error { return nil }

func reachableData(acPower, energyTotal, energyToday float64) *InverterData {
	return &InverterData{
		ACPower:     acPower,
		EnergyTotal: energyTotal,
		EnergyToday: energyToday,
		Status:      4,
		Reachable:   true,
	}
}

func unreachableData() *InverterData {
	return &InverterData{Reachable: false}
}

func TestFallbackUsePrimaryWhenReachable(t *testing.T) {
	primary := &controllableBackend{data: reachableData(1000, 23000000, math.NaN())}
	secondary := &controllableBackend{data: reachableData(900, 23000000, 5000)}

	fb := NewFallbackBackend(primary, secondary, testLogger())

	data, err := fb.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ACPower != 1000 {
		t.Errorf("expected primary ACPower=1000, got %v", data.ACPower)
	}
	if fb.IsUsingFallback() {
		t.Error("should not be using fallback when primary is reachable")
	}
}

func TestFallbackSwitchesToSecondaryOnUnreachable(t *testing.T) {
	primary := &controllableBackend{data: unreachableData()}
	secondary := &controllableBackend{data: reachableData(900, 23000000, 5000)}

	fb := NewFallbackBackend(primary, secondary, testLogger())

	data, err := fb.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ACPower != 900 {
		t.Errorf("expected secondary ACPower=900, got %v", data.ACPower)
	}
	if !fb.IsUsingFallback() {
		t.Error("should be using fallback when primary is unreachable")
	}
}

func TestFallbackSwitchesToSecondaryOnError(t *testing.T) {
	primary := &controllableBackend{err: errors.New("i/o timeout")}
	secondary := &controllableBackend{data: reachableData(900, 23000000, 5000)}

	fb := NewFallbackBackend(primary, secondary, testLogger())

	data, err := fb.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ACPower != 900 {
		t.Errorf("expected secondary ACPower=900, got %v", data.ACPower)
	}
	if !fb.IsUsingFallback() {
		t.Error("should be using fallback on primary error")
	}
}

func TestFallbackRecovery(t *testing.T) {
	primary := &controllableBackend{data: unreachableData()}
	secondary := &controllableBackend{data: reachableData(900, 23000000, 5000)}

	fb := NewFallbackBackend(primary, secondary, testLogger())

	// First read: primary fails, switch to fallback
	fb.Read() //nolint:errcheck
	if !fb.IsUsingFallback() {
		t.Fatal("expected fallback after primary failure")
	}

	// Primary recovers
	primary.data = reachableData(1000, 23000000, math.NaN())
	primary.err = nil

	data, err := fb.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ACPower != 1000 {
		t.Errorf("expected primary ACPower=1000 after recovery, got %v", data.ACPower)
	}
	if fb.IsUsingFallback() {
		t.Error("should not be using fallback after primary recovery")
	}
}

func TestFallbackEnrichesEnergyTodayFromSecondary(t *testing.T) {
	// Modbus (primary) does not provide EnergyToday (it's NaN)
	primary := &controllableBackend{data: reachableData(1000, 23000000, math.NaN())}
	// API (secondary) has EnergyToday cached
	secondary := &controllableBackend{data: reachableData(900, 23000000, 5000)}

	fb := NewFallbackBackend(primary, secondary, testLogger())

	data, err := fb.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.IsNaN(data.EnergyToday) {
		t.Error("EnergyToday should be enriched from secondary, not NaN")
	}
	if data.EnergyToday != 5000 {
		t.Errorf("expected EnergyToday=5000 from secondary cache, got %v", data.EnergyToday)
	}
	// Other fields should still come from primary
	if data.ACPower != 1000 {
		t.Errorf("ACPower should still be from primary, got %v", data.ACPower)
	}
}

func TestFallbackDoesNotOverwriteEnergyTodayWhenPresent(t *testing.T) {
	// If primary somehow provides EnergyToday, don't overwrite it
	primary := &controllableBackend{data: reachableData(1000, 23000000, 3000)}
	secondary := &controllableBackend{data: reachableData(900, 23000000, 5000)}

	fb := NewFallbackBackend(primary, secondary, testLogger())

	data, err := fb.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.EnergyToday != 3000 {
		t.Errorf("expected primary EnergyToday=3000, got %v (secondary value should not overwrite)", data.EnergyToday)
	}
}

func TestFallbackActiveMetricInCollector(t *testing.T) {
	primary := &controllableBackend{data: unreachableData()}
	secondary := &controllableBackend{data: reachableData(900, 23000000, 5000)}

	fb := NewFallbackBackend(primary, secondary, testLogger())
	c := NewCollector(fb, nil)

	// Trigger a read so fallback state is updated
	output := collectMetrics(t, c)

	if !strings.Contains(output, "solaredge_fallback_active 1") {
		t.Errorf("expected fallback_active=1 in output:\n%s", output)
	}

	// Primary recovers
	primary.data = reachableData(1000, 23000000, math.NaN())
	output = collectMetrics(t, c)

	if !strings.Contains(output, "solaredge_fallback_active 0") {
		t.Errorf("expected fallback_active=0 after recovery:\n%s", output)
	}
}
