package main

import (
	"log/slog"
	"sync"
)

// FallbackBackend uses Modbus as the primary backend and falls back to the
// SolarEdge cloud API when the inverter is unreachable via Modbus.
// It switches back to Modbus automatically as soon as it becomes reachable again.
//
// Modbus already implements its own 60-second cooldown on failure, so no
// connection-timeout penalty is paid on every scrape during a failure window.
type FallbackBackend struct {
	primary   *ModbusBackend
	secondary *APIBackend
	logger    *slog.Logger

	mu            sync.Mutex
	usingFallback bool
}

func NewFallbackBackend(primary *ModbusBackend, secondary *APIBackend, logger *slog.Logger) *FallbackBackend {
	return &FallbackBackend{
		primary:   primary,
		secondary: secondary,
		logger:    logger,
	}
}

// Read tries Modbus first. If Modbus returns unreachable (or an error), it
// returns the latest cached data from the API backend instead.
func (f *FallbackBackend) Read() (*InverterData, error) {
	data, err := f.primary.Read()
	if err == nil && data.Reachable {
		f.mu.Lock()
		wasUsingFallback := f.usingFallback
		f.usingFallback = false
		f.mu.Unlock()

		if wasUsingFallback {
			f.logger.Info("modbus recovered, switched back from API fallback")
		}
		return data, nil
	}

	// Modbus unavailable — use API fallback
	f.mu.Lock()
	alreadyLogged := f.usingFallback
	f.usingFallback = true
	f.mu.Unlock()

	if !alreadyLogged {
		if err != nil {
			f.logger.Warn("modbus error, switching to API fallback", "error", err)
		} else {
			f.logger.Warn("modbus unreachable, switching to API fallback")
		}
	}

	return f.secondary.Read()
}

// IsUsingFallback reports whether the API fallback is currently active.
func (f *FallbackBackend) IsUsingFallback() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.usingFallback
}

func (f *FallbackBackend) Close() error {
	f.primary.Close()
	return f.secondary.Close()
}
