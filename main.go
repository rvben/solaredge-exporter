package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version = "dev"

const defaultAPIBaseURL = "https://monitoringapi.solaredge.com"

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func envOrDefaultDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return fallback
}

func main() {
	// Common flags
	backendFlag := flag.String("backend", envOrDefault("SE_BACKEND", "modbus"), "Backend type: modbus or api")
	listenFlag := flag.String("listen", envOrDefault("SE_LISTEN", ":2112"), "HTTP listen address")
	logLevelFlag := flag.String("log-level", envOrDefault("SE_LOG_LEVEL", "info"), "Log level: debug, info, warn, error")

	// Snapshot flags
	snapshotFile := flag.String("snapshot-file", envOrDefault("SE_SNAPSHOT_FILE", ""), "Path to daily energy snapshot file (enables today/month/year metrics)")
	timezoneFlag := flag.String("timezone", envOrDefault("TZ", "UTC"), "Timezone for calendar-period energy calculations")

	// Modbus flags (env vars used as defaults, flags take precedence)
	modbusAddr := flag.String("modbus-address", envOrDefault("SE_MODBUS_ADDRESS", ""), "Modbus TCP address (host:port)")
	modbusDevID := flag.Int("modbus-device-id", envOrDefaultInt("SE_MODBUS_DEVICE_ID", 1), "Modbus device ID")
	modbusTimeout := flag.Duration("modbus-timeout", envOrDefaultDuration("SE_MODBUS_TIMEOUT", 5*time.Second), "Modbus read timeout")

	// API flags
	apiKey := flag.String("api-key", envOrDefault("SE_API_KEY", ""), "SolarEdge API key")
	siteID := flag.String("site-id", envOrDefault("SE_SITE_ID", ""), "SolarEdge site ID")
	apiInterval := flag.Duration("api-interval", envOrDefaultDuration("SE_API_INTERVAL", 5*time.Minute), "API polling interval")

	flag.Parse()

	// Setup logging
	var level slog.Level
	switch *logLevelFlag {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	logger.Info("starting solaredge-exporter", "version", version, "backend", *backendFlag, "listen", *listenFlag)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create backend
	var backend Backend
	var err error
	switch *backendFlag {
	case "modbus":
		if *modbusAddr == "" {
			logger.Error("--modbus-address is required for modbus backend")
			os.Exit(1)
		}
		backend, err = NewModbusBackend(*modbusAddr, byte(*modbusDevID), *modbusTimeout, logger)
	case "api":
		if *apiKey == "" || *siteID == "" {
			logger.Error("--api-key and --site-id are required for api backend")
			os.Exit(1)
		}
		backend, err = NewAPIBackend(ctx, defaultAPIBaseURL, *apiKey, *siteID, *apiInterval, logger)
	case "fallback":
		if *modbusAddr == "" {
			logger.Error("--modbus-address is required for fallback backend")
			os.Exit(1)
		}
		if *apiKey == "" || *siteID == "" {
			logger.Error("--api-key and --site-id are required for fallback backend")
			os.Exit(1)
		}
		var mb *ModbusBackend
		mb, err = NewModbusBackend(*modbusAddr, byte(*modbusDevID), *modbusTimeout, logger)
		if err != nil {
			logger.Error("failed to initialize modbus backend", "error", err)
			os.Exit(1)
		}
		var ab *APIBackend
		ab, err = NewAPIBackend(ctx, defaultAPIBaseURL, *apiKey, *siteID, *apiInterval, logger)
		if err != nil {
			logger.Error("failed to initialize API fallback backend", "error", err)
			os.Exit(1)
		}
		backend = NewFallbackBackend(mb, ab, logger)
	default:
		logger.Error("unknown backend", "backend", *backendFlag)
		os.Exit(1)
	}
	if err != nil {
		logger.Error("failed to initialize backend", "error", err)
		os.Exit(1)
	}
	defer backend.Close()

	// Create snapshot store (optional)
	var snapshot *SnapshotStore
	if *snapshotFile != "" {
		tz, tzErr := time.LoadLocation(*timezoneFlag)
		if tzErr != nil {
			logger.Error("invalid timezone", "timezone", *timezoneFlag, "error", tzErr)
			os.Exit(1)
		}
		snapshot, err = NewSnapshotStore(*snapshotFile, tz, logger)
		if err != nil {
			logger.Error("failed to initialize snapshot store", "error", err)
			os.Exit(1)
		}
	}

	// Create collector and register
	collector := NewCollector(backend, snapshot)
	collector.logger = logger
	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "solaredge-exporter %s\n\n/metrics - Prometheus metrics\n/health  - Health check\n", version)
	})

	server := &http.Server{
		Addr:    *listenFlag,
		Handler: mux,
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigCh
		logger.Info("shutting down")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Error("shutdown error", "error", err)
			}
	}()

	logger.Info("listening", "address", *listenFlag)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
