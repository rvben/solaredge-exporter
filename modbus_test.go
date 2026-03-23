package main

import (
	"math"
	"testing"
)

func TestApplyScaleFactor(t *testing.T) {
	tests := []struct {
		name     string
		raw      uint16
		sf       int16
		expected float64
	}{
		{"positive sf", 2300, 0, 2300.0},
		{"negative sf -1", 2300, -1, 230.0},
		{"negative sf -2", 50005, -3, 50.005},
		{"sf +1", 23, 1, 230.0},
		{"zero raw", 0, -2, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyScaleFactor(tt.raw, tt.sf)
			if math.Abs(got-tt.expected) > 0.001 {
				t.Errorf("applyScaleFactor(%d, %d) = %f, want %f", tt.raw, tt.sf, got, tt.expected)
			}
		})
	}
}

func TestApplyScaleFactorSigned(t *testing.T) {
	tests := []struct {
		name     string
		raw      int16
		sf       int16
		expected float64
	}{
		{"positive", 2645, 0, 2645.0},
		{"negative value", -100, -1, -10.0},
		{"with scale", 437, -1, 43.7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyScaleFactorSigned(tt.raw, tt.sf)
			if math.Abs(got-tt.expected) > 0.001 {
				t.Errorf("applyScaleFactorSigned(%d, %d) = %f, want %f", tt.raw, tt.sf, got, tt.expected)
			}
		})
	}
}

func TestIsSentinel(t *testing.T) {
	tests := []struct {
		name     string
		val      uint16
		expected bool
	}{
		{"normal value", 1234, false},
		{"sentinel uint16", 0xFFFF, true},
		{"sentinel int16", 0x7FFF, true},
		{"zero", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSentinelU16(tt.val)
			if got != tt.expected {
				t.Errorf("isSentinelU16(%d) = %v, want %v", tt.val, got, tt.expected)
			}
		})
	}
}

func TestIsSentinelU32(t *testing.T) {
	if !isSentinelU32(0xFFFFFFFF) {
		t.Error("expected 0xFFFFFFFF to be sentinel")
	}
	if isSentinelU32(22948250) {
		t.Error("expected 22948250 to not be sentinel")
	}
}

func TestToUint32(t *testing.T) {
	hi := uint16(0x015E)
	lo := uint16(0x56DA)
	got := toUint32(hi, lo)
	expected := uint32(0x015E56DA)
	if got != expected {
		t.Errorf("toUint32(%d, %d) = %d, want %d", hi, lo, got, expected)
	}
}

func TestRegsToString(t *testing.T) {
	regs := []uint16{0x536F, 0x6C61, 0x7245, 0x6467, 0x6500, 0x0000}
	got := regsToString(regs)
	if got != "SolarEdge" {
		t.Errorf("regsToString() = %q, want %q", got, "SolarEdge")
	}
}

func sfReg(sf int16) uint16 { return uint16(sf) }

func TestParseInverterBlock(t *testing.T) {
	regs := make([]uint16, 37)
	regs[0] = 1154         // AC Current
	regs[4] = sfReg(-2)    // AC Current SF
	regs[8] = 2294         // AC Voltage
	regs[11] = sfReg(-1)   // AC Voltage SF
	regs[12] = 2645        // AC Power
	regs[13] = 0           // AC Power SF
	regs[14] = 50005       // AC Frequency
	regs[15] = sfReg(-3)   // AC Frequency SF
	regs[22] = 0x015E      // Energy Total hi
	regs[23] = 0x56DA      // Energy Total lo
	regs[24] = 0           // Energy SF
	regs[25] = 535         // DC Current
	regs[26] = sfReg(-2)   // DC Current SF
	regs[27] = 3699        // DC Voltage
	regs[28] = sfReg(-1)   // DC Voltage SF
	regs[29] = 2686        // DC Power
	regs[30] = 0           // DC Power SF
	regs[32] = 437         // Temperature
	regs[35] = sfReg(-1)   // Temperature SF
	regs[36] = 4           // Status (Producing)

	data := parseInverterBlock(regs)

	assertClose(t, "ACCurrent", data.ACCurrent, 11.54)
	assertClose(t, "ACVoltage", data.ACVoltage, 229.4)
	assertClose(t, "ACPower", data.ACPower, 2645.0)
	assertClose(t, "ACFrequency", data.ACFrequency, 50.005)
	assertClose(t, "EnergyTotal", data.EnergyTotal, 22959834.0)
	assertClose(t, "DCCurrent", data.DCCurrent, 5.35)
	assertClose(t, "DCVoltage", data.DCVoltage, 369.9)
	assertClose(t, "DCPower", data.DCPower, 2686.0)
	assertClose(t, "Temperature", data.Temperature, 43.7)
	if data.Status != 4 {
		t.Errorf("Status = %d, want 4", data.Status)
	}
}

func TestParseInverterBlockSentinelsUint16(t *testing.T) {
	regs := make([]uint16, 37)
	for i := range regs {
		regs[i] = 0xFFFF
	}
	data := parseInverterBlock(regs)
	if !math.IsNaN(data.ACPower) {
		t.Errorf("ACPower should be NaN for sentinel, got %f", data.ACPower)
	}
	if !math.IsNaN(data.ACCurrent) {
		t.Errorf("ACCurrent should be NaN for sentinel, got %f", data.ACCurrent)
	}
	if !math.IsNaN(data.DCVoltage) {
		t.Errorf("DCVoltage should be NaN for sentinel, got %f", data.DCVoltage)
	}
	if !math.IsNaN(data.EnergyTotal) {
		t.Errorf("EnergyTotal should be NaN for sentinel, got %f", data.EnergyTotal)
	}
	if !math.IsNaN(data.Temperature) {
		t.Errorf("Temperature should be NaN for 0xFFFF sentinel, got %f", data.Temperature)
	}
}

func TestParseInverterBlockSentinelInt16(t *testing.T) {
	regs := make([]uint16, 37)
	regs[32] = 0x7FFF    // Temperature int16 sentinel
	regs[35] = sfReg(-1) // Temperature SF
	regs[12] = 1000      // AC Power normal value
	regs[13] = 0         // AC Power SF

	data := parseInverterBlock(regs)
	if !math.IsNaN(data.Temperature) {
		t.Errorf("Temperature should be NaN for 0x7FFF sentinel, got %f", data.Temperature)
	}
	assertClose(t, "ACPower", data.ACPower, 1000.0)
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %f, want %f", name, got, want)
	}
}
