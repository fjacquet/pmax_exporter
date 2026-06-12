# pmax_exporter

[![CI](https://github.com/fjacquet/pmax_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/pmax_exporter/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/fjacquet/pmax_exporter?include_prereleases&sort=semver&logo=github)](https://github.com/fjacquet/pmax_exporter/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/fjacquet/pmax_exporter)](https://goreportcard.com/report/github.com/fjacquet/pmax_exporter)
[![Go version](https://img.shields.io/github/go-mod/go-version/fjacquet/pmax_exporter?logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Docs](https://github.com/fjacquet/pmax_exporter/actions/workflows/docs.yml/badge.svg)](https://fjacquet.github.io/pmax_exporter/)
[![Container image](https://img.shields.io/badge/GHCR-pmax__exporter-2496ED?logo=docker&logoColor=white)](https://github.com/fjacquet/pmax_exporter/pkgs/container/pmax_exporter)

Prometheus + OTLP exporter for **Dell PowerMax**, via the Unisphere for PowerMax REST
API. One process monitors any number of Unisphere instances and arrays.

**Docs: <https://fjacquet.github.io/pmax_exporter/>**

## Features

- Array, FE/BE/RDF director, FE/BE port, cache partition, storage group, and SRP
  **performance** (diagnostic 5-min data), SRP **capacity**, array inventory — all as
  gauges with unit-explicit names ([metrics reference](docs/metrics.md)). Opt-in
  per-volume metrics, batched by storage group.
- **Grafana dashboard** included (`grafana/dashboards/pmax-overview.json`), provisioned
  automatically by the compose stack.
- **Snapshot model**: a background loop polls Unisphere; `/metrics` scrapes and OTLP
  pushes read an immutable snapshot — backend load is independent of scraper count.
- **Dual export**: Prometheus exposition + optional OTLP gRPC push.
- `server` + `array` identity labels on every metric; per-instance array allowlist.
- Config **hot reload** (SIGHUP + file watch), `${ENV}` interpolation, `passwordFile`,
  native `.env` loading.
- `--once --debug` sample dump and credential-safe `--trace` for live-array validation.

## Quick start

```bash
cp .env.example .env        # set PMAX1_HOSTNAME / PMAX1_USERNAME / PMAX1_PASSWORD
docker compose up -d        # exporter (:9104) + Prometheus (:9090) + Grafana (:3000)
curl -s localhost:9104/metrics | grep pmax_up
```

Or bare metal:

```bash
make cli && ./bin/pmax_exporter --config config.yaml --debug
```

Unisphere prerequisites: a read-only (Monitor) user and **diagnostic performance
registration** enabled per array.

## Development

```bash
make tools   # golangci-lint, govulncheck, cyclonedx-gomod
make ci      # fmt-check + vet + lint + test -race + govulncheck + build
make sure    # quick local gate
```

Architecture and design rationale live in [docs/adr/](docs/adr/index.md). The client is
hand-rolled `resty/v2` — `dell/gopowermax` was evaluated and declined because its
performance API coverage is CSI-scoped (ADR-0003).

## License

[MIT](LICENSE)
