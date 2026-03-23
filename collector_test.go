package main

import (
	"math"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// mockBackend implements Backend for testing.
type mockBackend struct {
	data *InverterData
	err  error
}

func (m *mockBackend) Read() (*InverterData, error) {
	return m.data, m.err
}

func (m *mockBackend) Close() error {
	return nil
}

func collectMetrics(t *testing.T, c *Collector) string {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	var sb strings.Builder
	enc := expfmt.NewEncoder(&sb, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, f := range families {
		if err := enc.Encode(f); err != nil {
			t.Fatalf("failed to encode metrics: %v", err)
		}
	}
	return sb.String()
}

func TestCollectorProducing(t *testing.T) {
	backend := &mockBackend{
		data: &InverterData{
			ACPower:      2645,
			DCPower:      2686,
			ACVoltage:    229.4,
			ACCurrent:    11.54,
			ACFrequency:  50.005,
			DCVoltage:    369.9,
			DCCurrent:    5.35,
			Temperature:  43.7,
			EnergyTotal:  22948250,
			Status:       4,
			Reachable:    true,
			Manufacturer: "SolarEdge",
			Model:        "SE4000H",
			Serial:       "12345",
			Version:      "1.0.0",
		},
	}

	c := NewCollector(backend)
	output := collectMetrics(t, c)

	expected := []string{
		"solaredge_ac_power_watts 2645",
		"solaredge_dc_power_watts 2686",
		"solaredge_ac_voltage_volts 229.4",
		"solaredge_ac_current_amps 11.54",
		"solaredge_temperature_celsius 43.7",
		"solaredge_energy_total_wh 2.294825e+07",
		"solaredge_status 4",
		"solaredge_inverter_reachable 1",
		`solaredge_info{manufacturer="SolarEdge",model="SE4000H",serial="12345",version="1.0.0"} 1`,
	}
	for _, exp := range expected {
		if !strings.Contains(output, exp) {
			t.Errorf("missing metric %q in output:\n%s", exp, output)
		}
	}
}

func TestCollectorSentinelOmission(t *testing.T) {
	backend := &mockBackend{
		data: &InverterData{
			ACPower:     math.NaN(),
			DCPower:     1000,
			ACVoltage:   math.NaN(),
			ACCurrent:   math.NaN(),
			ACFrequency: math.NaN(),
			DCVoltage:   350,
			DCCurrent:   3.0,
			Temperature: math.NaN(),
			EnergyTotal: math.NaN(),
			Status:      4,
			Reachable:   true,
		},
	}

	c := NewCollector(backend)
	output := collectMetrics(t, c)

	// These should NOT appear (NaN = sentinel = omit)
	omitted := []string{
		"solaredge_ac_power_watts",
		"solaredge_ac_voltage_volts",
		"solaredge_ac_current_amps",
		"solaredge_temperature_celsius",
		"solaredge_energy_total_wh",
	}
	for _, m := range omitted {
		if strings.Contains(output, m) {
			t.Errorf("metric %q should be omitted for NaN, but found in output:\n%s", m, output)
		}
	}

	// These should still appear
	if !strings.Contains(output, "solaredge_dc_power_watts 1000") {
		t.Error("dc_power should be present")
	}
}

func TestCollectorUnreachable(t *testing.T) {
	backend := &mockBackend{
		data: &InverterData{Reachable: false},
	}

	c := NewCollector(backend)
	output := collectMetrics(t, c)

	if !strings.Contains(output, "solaredge_inverter_reachable 0") {
		t.Errorf("expected reachable=0, got:\n%s", output)
	}
}
