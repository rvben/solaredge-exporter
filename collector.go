package main

import (
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector implements prometheus.Collector for SolarEdge inverter data.
type Collector struct {
	backend Backend
	logger  *slog.Logger

	// Metric descriptors
	acPower     *prometheus.Desc
	dcPower     *prometheus.Desc
	acVoltage   *prometheus.Desc
	acCurrent   *prometheus.Desc
	acFrequency *prometheus.Desc
	dcVoltage   *prometheus.Desc
	dcCurrent   *prometheus.Desc
	temperature *prometheus.Desc
	energyTotal *prometheus.Desc
	status      *prometheus.Desc
	reachable   *prometheus.Desc
	info        *prometheus.Desc

	scrapeDuration *prometheus.Desc
	scrapeErrors   prometheus.Counter

	mu           sync.Mutex
	lastStatus   uint16
	statusLogged bool
}

func NewCollector(backend Backend) *Collector {
	return &Collector{
		backend: backend,
		logger:  slog.Default(),
		acPower: prometheus.NewDesc("solaredge_ac_power_watts",
			"AC power output in watts", nil, nil),
		dcPower: prometheus.NewDesc("solaredge_dc_power_watts",
			"DC power input from panels in watts", nil, nil),
		acVoltage: prometheus.NewDesc("solaredge_ac_voltage_volts",
			"AC voltage", nil, nil),
		acCurrent: prometheus.NewDesc("solaredge_ac_current_amps",
			"AC current", nil, nil),
		acFrequency: prometheus.NewDesc("solaredge_ac_frequency_hertz",
			"Grid frequency", nil, nil),
		dcVoltage: prometheus.NewDesc("solaredge_dc_voltage_volts",
			"DC voltage from panels", nil, nil),
		dcCurrent: prometheus.NewDesc("solaredge_dc_current_amps",
			"DC current from panels", nil, nil),
		temperature: prometheus.NewDesc("solaredge_temperature_celsius",
			"Inverter heat sink temperature", nil, nil),
		energyTotal: prometheus.NewDesc("solaredge_energy_total_wh",
			"Lifetime energy production in watt-hours", nil, nil),
		status: prometheus.NewDesc("solaredge_status",
			"Inverter operating status (1=Off, 2=Sleeping, 3=Starting, 4=Producing, 5=Throttled, 6=Shutting down, 7=Fault)", nil, nil),
		reachable: prometheus.NewDesc("solaredge_inverter_reachable",
			"Whether the inverter is reachable (1=yes, 0=no)", nil, nil),
		info: prometheus.NewDesc("solaredge_info",
			"Inverter identity information",
			[]string{"manufacturer", "model", "serial", "version"}, nil),
		scrapeDuration: prometheus.NewDesc("solaredge_scrape_duration_seconds",
			"Time taken to read from backend", nil, nil),
		scrapeErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "solaredge_scrape_errors_total",
			Help: "Number of failed backend reads",
		}),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acPower
	ch <- c.dcPower
	ch <- c.acVoltage
	ch <- c.acCurrent
	ch <- c.acFrequency
	ch <- c.dcVoltage
	ch <- c.dcCurrent
	ch <- c.temperature
	ch <- c.energyTotal
	ch <- c.status
	ch <- c.reachable
	ch <- c.info
	ch <- c.scrapeDuration
	c.scrapeErrors.Describe(ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	data, err := c.backend.Read()

	duration := time.Since(start).Seconds()
	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, duration)
	c.scrapeErrors.Collect(ch)

	if err != nil {
		c.scrapeErrors.Inc()
		c.logger.Error("scrape failed", "error", err)
		ch <- prometheus.MustNewConstMetric(c.reachable, prometheus.GaugeValue, 0)
		return
	}

	if !data.Reachable {
		ch <- prometheus.MustNewConstMetric(c.reachable, prometheus.GaugeValue, 0)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.reachable, prometheus.GaugeValue, 1)

	// Only emit metrics that are not NaN (SunSpec sentinel)
	if !math.IsNaN(data.ACPower) {
		ch <- prometheus.MustNewConstMetric(c.acPower, prometheus.GaugeValue, data.ACPower)
	}
	if !math.IsNaN(data.DCPower) {
		ch <- prometheus.MustNewConstMetric(c.dcPower, prometheus.GaugeValue, data.DCPower)
	}
	if !math.IsNaN(data.ACVoltage) {
		ch <- prometheus.MustNewConstMetric(c.acVoltage, prometheus.GaugeValue, data.ACVoltage)
	}
	if !math.IsNaN(data.ACCurrent) {
		ch <- prometheus.MustNewConstMetric(c.acCurrent, prometheus.GaugeValue, data.ACCurrent)
	}
	if !math.IsNaN(data.ACFrequency) {
		ch <- prometheus.MustNewConstMetric(c.acFrequency, prometheus.GaugeValue, data.ACFrequency)
	}
	if !math.IsNaN(data.DCVoltage) {
		ch <- prometheus.MustNewConstMetric(c.dcVoltage, prometheus.GaugeValue, data.DCVoltage)
	}
	if !math.IsNaN(data.DCCurrent) {
		ch <- prometheus.MustNewConstMetric(c.dcCurrent, prometheus.GaugeValue, data.DCCurrent)
	}
	if !math.IsNaN(data.Temperature) {
		ch <- prometheus.MustNewConstMetric(c.temperature, prometheus.GaugeValue, data.Temperature)
	}
	if !math.IsNaN(data.EnergyTotal) {
		ch <- prometheus.MustNewConstMetric(c.energyTotal, prometheus.GaugeValue, data.EnergyTotal)
	}

	ch <- prometheus.MustNewConstMetric(c.status, prometheus.GaugeValue, float64(data.Status))

	if data.Manufacturer != "" {
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
			data.Manufacturer, data.Model, data.Serial, data.Version)
	}

	c.logStatusTransition(data.Status)
}

func (c *Collector) logStatusTransition(status uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if status == c.lastStatus && c.statusLogged {
		return
	}

	c.lastStatus = status
	c.statusLogged = true

	switch status {
	case 1, 2:
		c.logger.Info("inverter sleeping", "status", status)
	case 4:
		c.logger.Info("inverter producing", "status", status)
	case 7:
		c.logger.Warn("inverter fault", "status", status)
	}
}
