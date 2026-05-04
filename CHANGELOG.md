# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).



## [0.3.4](https://github.com/rvben/solaredge-exporter/compare/v0.3.3...v0.3.4) - 2026-05-04

### Fixed

- **snapshot**: skip back-calc at midnight rollover ([8f25344](https://github.com/rvben/solaredge-exporter/commit/8f25344ed1bbcd362d16e5afd379ba2a412fe9ed))

## [0.3.3](https://github.com/rvben/solaredge-exporter/compare/v0.3.2...v0.3.3) - 2026-04-07

### Performance

- **modbus**: reduce timeout to 3s and increase cooldown to 90s ([f679e8c](https://github.com/rvben/solaredge-exporter/commit/f679e8c10bcb95af7880f17d15d111107dd51128))

## [0.2.0](https://github.com/rvben/solaredge-exporter/compare/v0.1.0...v0.2.0) - 2026-03-31

### Fixed

- **snapshot**: back-calculate midnight baseline on mid-day restart ([37c94cc](https://github.com/rvben/solaredge-exporter/commit/37c94ccc4f19d592ca5f4da55ccac08dc593cec6))
