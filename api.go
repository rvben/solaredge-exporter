package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"
)

type apiOverviewResponse struct {
	Overview struct {
		LastUpdateTime string `json:"lastUpdateTime"`
		CurrentPower   struct {
			Power float64 `json:"power"`
		} `json:"currentPower"`
		LifeTimeData struct {
			Energy float64 `json:"energy"`
		} `json:"lifeTimeData"`
		LastDayData struct {
			Energy float64 `json:"energy"`
		} `json:"lastDayData"`
	} `json:"overview"`
}

func (r *apiOverviewResponse) toInverterData() InverterData {
	return InverterData{
		ACPower:     r.Overview.CurrentPower.Power,
		DCPower:     math.NaN(), // Not available from API overview
		ACVoltage:   math.NaN(),
		ACCurrent:   math.NaN(),
		ACFrequency: math.NaN(),
		DCVoltage:   math.NaN(),
		DCCurrent:   math.NaN(),
		Temperature: math.NaN(),
		EnergyTotal: r.Overview.LifeTimeData.Energy,
		EnergyToday: r.Overview.LastDayData.Energy,
		Status:      4, // API doesn't expose SunSpec status; assume producing if responding
		Reachable:   true,
	}
}

// APIBackend reads from the SolarEdge Monitoring API.
type APIBackend struct {
	baseURL  string
	apiKey   string
	siteID   string
	interval time.Duration
	logger   *slog.Logger
	client   *http.Client

	mu     sync.RWMutex
	cached *InverterData
	cancel context.CancelFunc
}

func NewAPIBackend(ctx context.Context, baseURL, apiKey, siteID string, interval time.Duration, logger *slog.Logger) (*APIBackend, error) {
	if logger == nil {
		logger = slog.Default()
	}

	ab := &APIBackend{
		baseURL:  baseURL,
		apiKey:   apiKey,
		siteID:   siteID,
		interval: interval,
		logger:   logger,
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	// Initial synchronous fetch
	data, err := ab.fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("initial API fetch: %w", err)
	}
	ab.cached = data

	// Start background polling
	pollCtx, cancel := context.WithCancel(ctx)
	ab.cancel = cancel
	go ab.poll(pollCtx)

	return ab, nil
}

func (ab *APIBackend) fetch(ctx context.Context) (*InverterData, error) {
	url := fmt.Sprintf("%s/site/%s/overview?api_key=%s", ab.baseURL, ab.siteID, ab.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := ab.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (429)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var apiResp apiOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	data := apiResp.toInverterData()
	return &data, nil
}

func (ab *APIBackend) poll(ctx context.Context) {
	ticker := time.NewTicker(ab.interval)
	defer ticker.Stop()

	wasReachable := true

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := ab.fetch(ctx)
			if err != nil {
				ab.mu.Lock()
				if ab.cached != nil {
					ab.cached.Reachable = false
				}
				ab.mu.Unlock()

				if wasReachable {
					ab.logger.Warn("API unreachable", "error", err)
					wasReachable = false
				} else {
					ab.logger.Debug("API still unreachable", "error", err)
				}

				// Handle 429: double interval for one cycle then reset on next tick
				if err.Error() == "rate limited (429)" {
					ab.logger.Warn("rate limited, backing off", "next_interval", ab.interval*2)
					ticker.Reset(ab.interval * 2)
				}
				continue
			}

			// Success: update cache and reset ticker to normal interval
			ab.mu.Lock()
			ab.cached = data
			ab.mu.Unlock()

			if !wasReachable {
				ab.logger.Info("API back online")
				wasReachable = true
			}

			// Reset ticker to normal interval (in case it was doubled after a 429)
			ticker.Reset(ab.interval)
		}
	}
}

func (ab *APIBackend) Read() (*InverterData, error) {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	if ab.cached == nil {
		return &InverterData{Reachable: false}, nil
	}
	// Return a copy to avoid data races
	data := *ab.cached
	return &data, nil
}

func (ab *APIBackend) Close() error {
	if ab.cancel != nil {
		ab.cancel()
	}
	return nil
}
