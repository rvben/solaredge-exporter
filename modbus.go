package main

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/goburrow/modbus"
)

const (
	regInverterStart = 40071
	regInverterCount = 37

	offACCurrent   = 0
	offACCurrentSF = 4
	offACVoltage   = 8
	offACVoltageSF = 11
	offACPower     = 12
	offACPowerSF   = 13
	offACFrequency = 14
	offACFreqSF    = 15
	offEnergyHi    = 22
	offEnergyLo    = 23
	offEnergySF    = 24
	offDCCurrent   = 25
	offDCCurrentSF = 26
	offDCVoltage   = 27
	offDCVoltageSF = 28
	offDCPower     = 29
	offDCPowerSF   = 30
	offTemperature = 32
	offTempSF      = 35
	offStatus      = 36

	regManufacturer = 40005
	regModel        = 40021
	regVersion      = 40037
	regSerial       = 40053
	regStringCount  = 16

	cooldownDuration = 90 * time.Second
)

func applyScaleFactor(raw uint16, sf int16) float64 {
	return float64(raw) * math.Pow(10, float64(sf))
}

func applyScaleFactorSigned(raw int16, sf int16) float64 {
	return float64(raw) * math.Pow(10, float64(sf))
}

func isSentinelU16(val uint16) bool {
	return val == 0xFFFF || val == 0x7FFF
}

func isSentinelU32(val uint32) bool {
	return val == 0xFFFFFFFF
}

func toUint32(hi, lo uint16) uint32 {
	return uint32(hi)<<16 | uint32(lo)
}

func regsToString(regs []uint16) string {
	buf := make([]byte, len(regs)*2)
	for i, r := range regs {
		binary.BigEndian.PutUint16(buf[i*2:], r)
	}
	// Trim null bytes from both ends
	start := 0
	for start < len(buf) && buf[start] == 0 {
		start++
	}
	end := len(buf)
	for end > start && buf[end-1] == 0 {
		end--
	}
	return strings.TrimSpace(string(buf[start:end]))
}

func parseInverterBlock(regs []uint16) InverterData {
	var data InverterData
	data.Reachable = true
	data.EnergyToday = math.NaN() // not available via Modbus

	if isSentinelU16(regs[offACCurrent]) {
		data.ACCurrent = math.NaN()
	} else {
		data.ACCurrent = applyScaleFactor(regs[offACCurrent], int16(regs[offACCurrentSF]))
	}

	if isSentinelU16(regs[offACVoltage]) {
		data.ACVoltage = math.NaN()
	} else {
		data.ACVoltage = applyScaleFactor(regs[offACVoltage], int16(regs[offACVoltageSF]))
	}

	if isSentinelU16(regs[offACPower]) {
		data.ACPower = math.NaN()
	} else {
		data.ACPower = applyScaleFactorSigned(int16(regs[offACPower]), int16(regs[offACPowerSF]))
	}

	if isSentinelU16(regs[offACFrequency]) {
		data.ACFrequency = math.NaN()
	} else {
		data.ACFrequency = applyScaleFactor(regs[offACFrequency], int16(regs[offACFreqSF]))
	}

	energyRaw := toUint32(regs[offEnergyHi], regs[offEnergyLo])
	if isSentinelU32(energyRaw) {
		data.EnergyTotal = math.NaN()
	} else {
		data.EnergyTotal = float64(energyRaw) * math.Pow(10, float64(int16(regs[offEnergySF])))
	}

	if isSentinelU16(regs[offDCCurrent]) {
		data.DCCurrent = math.NaN()
	} else {
		data.DCCurrent = applyScaleFactor(regs[offDCCurrent], int16(regs[offDCCurrentSF]))
	}

	if isSentinelU16(regs[offDCVoltage]) {
		data.DCVoltage = math.NaN()
	} else {
		data.DCVoltage = applyScaleFactor(regs[offDCVoltage], int16(regs[offDCVoltageSF]))
	}

	if isSentinelU16(regs[offDCPower]) {
		data.DCPower = math.NaN()
	} else {
		data.DCPower = applyScaleFactorSigned(int16(regs[offDCPower]), int16(regs[offDCPowerSF]))
	}

	if isSentinelU16(regs[offTemperature]) {
		data.Temperature = math.NaN()
	} else {
		data.Temperature = applyScaleFactorSigned(int16(regs[offTemperature]), int16(regs[offTempSF]))
	}

	data.Status = regs[offStatus]
	return data
}

type ModbusBackend struct {
	address  string
	deviceID byte
	timeout  time.Duration
	logger   *slog.Logger

	manufacturer string
	model        string
	serial       string
	version      string

	mu            sync.Mutex
	lastFailure   time.Time
	cachedFailure bool
	wasReachable  bool
}

func NewModbusBackend(address string, deviceID byte, timeout time.Duration, logger *slog.Logger) (*ModbusBackend, error) {
	mb := &ModbusBackend{
		address:      address,
		deviceID:     deviceID,
		timeout:      timeout,
		logger:       logger,
		wasReachable: true,
	}

	client, handler, err := mb.connect()
	if err != nil {
		return nil, fmt.Errorf("connecting to inverter: %w", err)
	}
	defer func() { _ = handler.Close() }()

	mb.manufacturer, err = mb.readString(client, regManufacturer)
	if err != nil {
		return nil, fmt.Errorf("reading manufacturer: %w", err)
	}
	mb.model, err = mb.readString(client, regModel)
	if err != nil {
		return nil, fmt.Errorf("reading model: %w", err)
	}
	mb.version, err = mb.readString(client, regVersion)
	if err != nil {
		return nil, fmt.Errorf("reading version: %w", err)
	}
	mb.serial, err = mb.readString(client, regSerial)
	if err != nil {
		return nil, fmt.Errorf("reading serial: %w", err)
	}

	logger.Info("connected to inverter",
		"manufacturer", mb.manufacturer,
		"model", mb.model,
		"serial", mb.serial,
		"version", mb.version,
	)

	return mb, nil
}

func (mb *ModbusBackend) connect() (modbus.Client, *modbus.TCPClientHandler, error) {
	handler := modbus.NewTCPClientHandler(mb.address)
	handler.Timeout = mb.timeout
	handler.SlaveId = mb.deviceID
	if err := handler.Connect(); err != nil {
		return nil, nil, err
	}
	return modbus.NewClient(handler), handler, nil
}

// readString reads a string from the given SunSpec register address.
func (mb *ModbusBackend) readString(client modbus.Client, register uint16) (string, error) {
	results, err := client.ReadHoldingRegisters(register-1, regStringCount)
	if err != nil {
		return "", err
	}
	regs := bytesToUint16(results)
	return regsToString(regs), nil
}

func bytesToUint16(data []byte) []uint16 {
	regs := make([]uint16, len(data)/2)
	for i := range regs {
		regs[i] = binary.BigEndian.Uint16(data[i*2:])
	}
	return regs
}

func (mb *ModbusBackend) Read() (*InverterData, error) {
	mb.mu.Lock()
	if mb.cachedFailure && time.Since(mb.lastFailure) < cooldownDuration {
		mb.mu.Unlock()
		return &InverterData{Reachable: false}, nil
	}
	mb.mu.Unlock()

	client, handler, err := mb.connect()
	if err != nil {
		mb.handleFailure()
		return nil, fmt.Errorf("connecting to inverter: %w", err)
	}
	defer func() { _ = handler.Close() }()

	results, err := client.ReadHoldingRegisters(regInverterStart, regInverterCount)
	if err != nil {
		mb.handleFailure()
		return nil, fmt.Errorf("reading inverter registers: %w", err)
	}

	regs := bytesToUint16(results)
	data := parseInverterBlock(regs)
	data.Manufacturer = mb.manufacturer
	data.Model = mb.model
	data.Serial = mb.serial
	data.Version = mb.version

	mb.handleSuccess()
	return &data, nil
}

func (mb *ModbusBackend) handleFailure() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.wasReachable {
		mb.logger.Warn("inverter unreachable")
		mb.wasReachable = false
	} else {
		mb.logger.Debug("inverter still unreachable")
	}
	mb.cachedFailure = true
	mb.lastFailure = time.Now()
}

func (mb *ModbusBackend) handleSuccess() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if !mb.wasReachable {
		mb.logger.Info("inverter online")
	}
	mb.wasReachable = true
	mb.cachedFailure = false
}

func (mb *ModbusBackend) Close() error {
	return nil
}
