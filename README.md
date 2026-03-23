# solaredge-exporter

A lean Prometheus exporter for SolarEdge inverters. Reads data via **Modbus TCP** (real-time, local) or the **SolarEdge Monitoring API** (cloud, ~15 min delay).

Two external dependencies. Single binary. ~15MB Docker image.

## Features

- **Modbus TCP backend** — real-time data directly from the inverter over your local network
- **SolarEdge API backend** — no hardware access needed, uses the cloud monitoring API
- **SunSpec-compliant** — reads standard SunSpec register map (verified on SE4000H)
- **Sentinel handling** — SunSpec "not available" values are omitted from metrics, not reported as garbage
- **Smart status tracking** — distinguishes sleeping inverter (normal at night) from unreachable (a problem)
- **Graceful shutdown** — clean SIGTERM/SIGINT handling for Docker/Kubernetes

## Quickstart

### Binary

```bash
# Modbus TCP (real-time, requires network access to inverter)
solaredge-exporter --backend modbus --modbus-address 192.168.1.100:1502

# SolarEdge Monitoring API
solaredge-exporter --backend api --api-key YOUR_KEY --site-id YOUR_SITE_ID
```

### Docker

```bash
# Modbus TCP
docker run -d --name solaredge-exporter \
  -p 2112:2112 \
  ghcr.io/rvben/solaredge-exporter:latest \
  --backend modbus --modbus-address 192.168.1.100:1502

# SolarEdge API
docker run -d --name solaredge-exporter \
  -p 2112:2112 \
  -e SE_BACKEND=api \
  -e SE_API_KEY=your_key \
  -e SE_SITE_ID=your_site_id \
  ghcr.io/rvben/solaredge-exporter:latest
```

## Configuration

All options can be set via CLI flags or environment variables. Flags take precedence.

### Common

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--backend` | `SE_BACKEND` | `modbus` | Backend: `modbus` or `api` |
| `--listen` | `SE_LISTEN` | `:2112` | HTTP listen address |
| `--log-level` | `SE_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

### Modbus Backend

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--modbus-address` | `SE_MODBUS_ADDRESS` | *(required)* | Inverter address (`host:port`) |
| `--modbus-device-id` | `SE_MODBUS_DEVICE_ID` | `1` | Modbus device ID |
| `--modbus-timeout` | `SE_MODBUS_TIMEOUT` | `5s` | Read timeout |

### API Backend

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--api-key` | `SE_API_KEY` | *(required)* | SolarEdge monitoring API key |
| `--site-id` | `SE_SITE_ID` | *(required)* | SolarEdge site ID |
| `--api-interval` | `SE_API_INTERVAL` | `5m` | Polling interval |

## Enabling Modbus TCP on your inverter

Modbus TCP is disabled by default on SolarEdge inverters. To enable it:

1. Flip the **red toggle switch** on the bottom of the inverter to the **"P" position** for less than 5 seconds
2. Connect to the inverter's WiFi network (password is on a sticker on the right side)
3. Open `http://172.16.0.1` in a browser
4. Go to **Site Communication** and enable **Modbus TCP** (default port: 1502)

No installer account or SetApp access is needed for this.

## Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: solaredge
    scrape_interval: 5s
    static_configs:
      - targets: ['localhost:2112']
```

## Metrics

### Inverter Metrics

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `solaredge_ac_power_watts` | Gauge | W | AC power output |
| `solaredge_dc_power_watts` | Gauge | W | DC power input from panels |
| `solaredge_ac_voltage_volts` | Gauge | V | AC voltage |
| `solaredge_ac_current_amps` | Gauge | A | AC current |
| `solaredge_ac_frequency_hertz` | Gauge | Hz | Grid frequency |
| `solaredge_dc_voltage_volts` | Gauge | V | DC voltage from panels |
| `solaredge_dc_current_amps` | Gauge | A | DC current from panels |
| `solaredge_temperature_celsius` | Gauge | C | Inverter heat sink temperature |
| `solaredge_energy_total_wh` | Gauge | Wh | Lifetime energy production |
| `solaredge_status` | Gauge | - | Operating status (1-7, see below) |
| `solaredge_inverter_reachable` | Gauge | - | 1 if inverter responded, 0 if unreachable |

### Status Values

| Value | Meaning |
|-------|---------|
| 1 | Off |
| 2 | Sleeping |
| 3 | Starting |
| 4 | Producing |
| 5 | Throttled |
| 6 | Shutting down |
| 7 | Fault |

### Info Metric

| Metric | Labels | Description |
|--------|--------|-------------|
| `solaredge_info` | `manufacturer`, `model`, `serial`, `version` | Always 1. Inverter identity. |

### Operational Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `solaredge_scrape_duration_seconds` | Gauge | Time taken to read from backend |
| `solaredge_scrape_errors_total` | Counter | Number of failed reads |

Metrics whose registers return SunSpec sentinel values ("not available") are omitted from the output rather than reported as bogus numbers.

## Endpoints

| Path | Description |
|------|-------------|
| `/metrics` | Prometheus metrics |
| `/health` | Health check (returns `{"status":"ok"}`) |
| `/` | Landing page with links |

## API Backend Notes

The SolarEdge Monitoring API provides fewer metrics than Modbus:
- Only `ac_power_watts` and `energy_total_wh` are available
- All other metrics are omitted (DC power, voltage, current, temperature, frequency)
- Data is delayed ~15 minutes
- Rate limit: 300 requests/day. On 429, the exporter backs off to 2x the poll interval for one cycle.

To get an API key: log in to [monitoring.solaredge.com](https://monitoring.solaredge.com) > Admin > Site Access > API Access.

## Building from Source

```bash
# Requires Go 1.24+
make build

# Run tests
make test

# Build Docker image
make docker
```

## License

MIT
