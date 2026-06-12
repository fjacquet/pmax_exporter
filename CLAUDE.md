# CLAUDE.md

Guidance for Claude Code in this repository. Family standard: `/exporter-standards`
(prescriptive — don't re-derive family decisions here).

## Overview

`pmax_exporter` — Prometheus + OTLP exporter for Dell PowerMax via the Unisphere REST
API. Go 1.26.4, hand-rolled `resty/v2` client (ADR-0003: `gopowermax` declined — perf
coverage is CSI-scoped). Metric prefix `pmax_`, port **9104**.

## Commands

```bash
make ci               # the gate: fmt-check, vet, golangci-lint, test -race, govulncheck, build
make sure             # quick local gate
make cli              # build bin/pmax_exporter
make release-snapshot # GoReleaser dry-run
./bin/pmax_exporter --config config.yaml --once --debug   # sample dump (sorted exposition)
./bin/pmax_exporter --trace                               # body-only API tracing (credential-safe)
```

## Architecture

- `main.go` — cobra CLI, HTTP served **before** first collect (ADR-0008), SIGHUP +
  file-watch reload (ADR-0005), `/health` off the snapshot.
- `internal/pmaxclient` — Unisphere client: **HTTP Basic per request** (no token
  endpoint exists, ADR-0004), TLS ≥1.2, retry 5xx-only, `Get`/`Post`, `Mock` for tests.
- `internal/pmax` — snapshot model (ADR-0002): `collector.go` discovers arrays via
  `GET /performance/Array/keys` per instance, then runs `ResourceCollector`s;
  `SnapshotStore` RWMutex pointer-swap; dual export `prometheus.go` (unchecked
  collector) + `otlp.go` (observable gauges).
- **Generic perf engine** (`perf.go` + `catalog.go`): every Unisphere performance
  category is `POST /performance/{Cat}/keys` → one `POST /performance/{Cat}/metrics`
  per object (`startDate=endDate=lastAvailableDate`, `dataFormat:"Average"`), fanned
  out with errgroup `SetLimit(collection.maxConcurrent)`. Two-level categories
  (FE/BE ports) set `Parent` in the catalog — child keys are POSTed once per parent
  director. New categories = catalog entries, not new code.
- **Volume metrics are opt-in** (`volume.go`, `collection.volumeMetrics`) — one series
  set per device. Batched: one `POST /performance/Volume/metrics` per ≤10 storage
  groups (`storageGroups` comma-list), the only batched per-object path Unisphere has.
- `internal/config` — yaml + `${ENV}` fail-fast interpolation + `passwordFile` +
  `.env` (godotenv; real env always wins).

## Load-bearing constraints

- **Absent, never zero** (ADR-0009): unparseable/missing API fields yield absent
  samples. Inventory structs use pointer fields; the perf engine decodes tolerantly
  (`toFloat` accepts string-typed numbers).
- **Label-key consistency** (ADR-0006): one label-key set per metric family —
  `server` first, `array` on array-scoped families, exactly one object label.
- **Gauges only, no `rate()`** (ADR-0007): perf values are already per-second/averaged;
  names are unit-explicit and keep Unisphere-native units (MB/s, ms, TB, GB).
- **Catalog keys are exact-case** — one wrong key 400s the whole category query
  (visible as `pmax_collector_up{collector="perf_…"} 0`). The catalog is provisional
  until live-validated (ADR-0009); cross-check against PyU4V/pmaxperfpy before editing.
- **Never log credentials**: no resty `SetDebug`; `--trace` is a body-only
  `OnAfterResponse` hook with a test asserting no leak.
- Default collection interval 5m = diagnostic granularity; don't "optimize" it down.
- No inline semgrep/`nolint` suppressions — restructure instead.

## Testing

`go test -race ./...` — mock-driven (`pmaxclient.Mock`); collector assertions go through
**both** the Prometheus registry gather and the OTLP `ManualReader`. The client is tested
against `httptest.NewTLSServer` (auth header, 5xx-vs-4xx retry, trace no-leak).

## CI/CD

`ci.yml` (make ci + SBOM + semgrep), `release.yml` (GoReleaser + GHCR multi-arch image on
`v*` tags), `docs.yml` (mkdocs → Pages). Actions SHA-pinned with `# vX.Y.Z` comments;
`persist-credentials: false` everywhere; release builds with `cache: false`.
