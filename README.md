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
# Modbus TCP with energy snapshots
docker run -d --name solaredge-exporter \
  -p 2112:2112 \
  -v solaredge-data:/data \
  -e SE_MODBUS_ADDRESS=192.168.1.100:1502 \
  -e SE_SNAPSHOT_FILE=/data/snapshots.json \
  -e TZ=Europe/Amsterdam \
  ghcr.io/rvben/solaredge-exporter:latest

# SolarEdge API
docker run -d --name solaredge-exporter \
  -p 2112:2112 \
  -v solaredge-data:/data \
  -e SE_BACKEND=api \
  -e SE_API_KEY=your_key \
  -e SE_SITE_ID=your_site_id \
  -e SE_SNAPSHOT_FILE=/data/snapshots.json \
  -e TZ=Europe/Amsterdam \
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
| `--snapshot-file` | `SE_SNAPSHOT_FILE` | *(disabled)* | Path to daily energy snapshot file (enables today/month/year metrics) |
| `--timezone` | `TZ` | `UTC` | Timezone for calendar-period calculations (e.g., `Europe/Amsterdam`) |

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

### Calendar-Period Energy Metrics (requires `--snapshot-file`)

| Metric | Type | Unit | Description |
|--------|------|------|-------------|
| `solaredge_energy_today_wh` | Gauge | Wh | Energy produced since midnight local time |
| `solaredge_energy_month_wh` | Gauge | Wh | Energy produced since the 1st of the current month |
| `solaredge_energy_year_wh` | Gauge | Wh | Energy produced since January 1st |
| `solaredge_snapshot_age_seconds` | Gauge | s | Time since the most recent daily snapshot |

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

## Energy Snapshots

The `--snapshot-file` flag enables calendar-period energy metrics (`energy_today_wh`, `energy_month_wh`, `energy_year_wh`). The exporter saves the inverter's lifetime energy counter once per day to a JSON file:

```json
{
  "2026-01-01": 22364527,
  "2026-03-01": 22611459,
  "2026-03-24": 22952008
}
```

Each value is the `energy_total_wh` at the start of that day. Calendar-period energy is computed as: `current_total - snapshot_at_period_start`.

**Timezone matters.** Set `TZ` (or `--timezone`) to your local timezone so "today" resets at your midnight, not UTC midnight.

**Seeding.** On first run the snapshot file is created empty — `energy_today_wh` works immediately but `energy_month_wh` and `energy_year_wh` need a Jan 1st / month-start reference point. You can seed the file manually with a known lifetime value from the SolarEdge monitoring portal.

**Persistence.** Mount a Docker volume for the snapshot file. If the file is lost, the exporter starts fresh — `energy_today_wh` works again after the first reading, but month/year need to accumulate new baselines.

**Counter resets.** If the inverter's lifetime counter resets (firmware update, replacement), the exporter detects it, clears all snapshots, and starts fresh.

**Pruning.** Daily entries older than 90 days are pruned automatically, except for the 1st of each month and January 1st (needed for month/year calculations).

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
