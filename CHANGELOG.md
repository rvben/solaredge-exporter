# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [0.3.8](https://github.com/rvben/solaredge-exporter/compare/v0.3.7...v0.3.8) - 2026-11-09

### Fixed

- **modbus**: expose explicit local provenance without API fallback ([0c3f59d](https://github.com/rvben/solaredge-exporter/commit/0c3f59de361c1f79383a2662d60edcc9b2b23b3c))

## [0.3.7](https://github.com/rvben/solaredge-exporter/compare/v0.3.6...v0.3.7) - 2026-11-09

## [0.3.6](https://github.com/rvben/solaredge-exporter/compare/v0.3.5...v0.3.6) - 2026-11-09

### Fixed

- **modbus**: coalesce concurrent inverter reads ([7c24c73](https://github.com/rvben/solaredge-exporter/commit/7c24c7352f72f8510f56479e14cc3bee85f5c43d))

## [0.3.5](https://github.com/rvben/solaredge-exporter/compare/v0.3.4...v0.3.5) - 2026-10-24

### Fixed

- **modbus**: retry sooner after brief inverter outages ([9c02539](https://github.com/rvben/solaredge-exporter/commit/9c02539e75cc686aa314b1db475b2d3d0036481e))

## [0.3.4](https://github.com/rvben/solaredge-exporter/compare/v0.3.3...v0.3.4) - 2026-05-04

### Fixed

- **snapshot**: skip back-calc at midnight rollover ([8f25344](https://github.com/rvben/solaredge-exporter/commit/8f25344ed1bbcd362d16e5afd379ba2a412fe9ed))

## [0.3.3](https://github.com/rvben/solaredge-exporter/compare/v0.3.2...v0.3.3) - 2026-04-07

### Performance

- **modbus**: reduce timeout to 3s and increase cooldown to 90s ([f679e8c](https://github.com/rvben/solaredge-exporter/commit/f679e8c10bcb95af7880f17d15d111107dd51128))

## [0.2.0](https://github.com/rvben/solaredge-exporter/compare/v0.1.0...v0.2.0) - 2026-03-31

### Fixed

- **snapshot**: back-calculate midnight baseline on mid-day restart ([37c94cc](https://github.com/rvben/solaredge-exporter/commit/37c94ccc4f19d592ca5f4da55ccac08dc593cec6))
