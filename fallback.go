package main

import (
	"log/slog"
	"math"
	"sync"
)

// FallbackBackend uses a primary backend (Modbus) and falls back to a secondary
// backend (API) when the primary is unreachable. It switches back to the primary
// automatically as soon as it becomes reachable again.
//
// ModbusBackend already implements its own 60-second cooldown on failure, so no
// connection-timeout penalty is paid on every scrape during a failure window.
type FallbackBackend struct {
	primary   Backend
	secondary Backend
	logger    *slog.Logger

	mu            sync.Mutex
	usingFallback bool
}

func NewFallbackBackend(primary, secondary Backend, logger *slog.Logger) *FallbackBackend {
	return &FallbackBackend{
		primary:   primary,
		secondary: secondary,
		logger:    logger,
	}
}

// Read tries Modbus first. If Modbus returns unreachable (or an error), it
// returns the latest cached data from the API backend instead.
//
// When Modbus is active, EnergyToday is enriched from the API cache if Modbus
// doesn't provide it (Modbus has no EnergyToday register). This ensures the
// snapshot store can back-calculate the midnight baseline correctly on restart.
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

		// Modbus has no EnergyToday register. Inject it from the API cache so
		// the snapshot store can back-calculate the midnight baseline correctly.
		if math.IsNaN(data.EnergyToday) {
			if apiData, apiErr := f.secondary.Read(); apiErr == nil && apiData.Reachable && !math.IsNaN(apiData.EnergyToday) {
				data.EnergyToday = apiData.EnergyToday
			}
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
	if err := f.primary.Close(); err != nil {
		return err
	}
	return f.secondary.Close()
}
