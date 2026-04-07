package main

// InverterData holds all metrics read from a SolarEdge inverter.
type InverterData struct {
	ACPower     float64
	DCPower     float64
	ACVoltage   float64
	ACCurrent   float64
	ACFrequency float64
	DCVoltage   float64
	DCCurrent   float64
	Temperature float64
	EnergyTotal  float64 // lifetime Wh
	EnergyToday  float64 // today's Wh from API; NaN if not available
	Status       uint16  // SunSpec status enum (1-7)
	Reachable   bool
	Manufacturer string
	Model        string
	Serial       string
	Version      string
}

// Backend reads inverter data from a SolarEdge inverter.
type Backend interface {
	Read() (*InverterData, error)
	Close() error
}

