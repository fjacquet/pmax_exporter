# CLAUDE.md

Guidance for Claude Code in this repository. Family standard: `/exporter-standards`
(prescriptive — don't re-derive family decisions here).

## Overview

`pmax_exporter` — Prometheus + OTLP exporter for Dell PowerMax via the Unisphere REST
API. Go (version pinned in `go.mod`), hand-rolled `resty/v2` client (ADR-0003: `gopowermax` declined — perf
coverage is CSI-scoped). Metric prefix `pmax_`, port **9443**.

## Commands

```bash
make ci               # the gate: fmt-check, vet, golangci-lint, test -race, govulncheck, build
make sure             # quick local gate
make cli              # build bin/pmax_exporter
make release-snapshot # GoReleaser dry-run
git tag vX.Y.Z && git push origin vX.Y.Z   # release: GoReleaser + GHCR image via release.yml
docker compose up -d  # demo stack: exporter :9443, Prometheus :9090, Grafana :3000 (dashboards auto-provisioned)
./bin/pmax_exporter --config config.yaml --once --debug   # sample dump (sorted exposition)
./bin/pmax_exporter --trace                               # body-only API tracing (credential-safe)
```

## Architecture

- `main.go` — cobra CLI, HTTP served **before** first collect (ADR-0008), SIGHUP +
  file-watch reload (ADR-0005), `/health` off the snapshot.
- `internal/pmaxclient` — Unisphere client: **HTTP Basic per request** (no token
  endpoint exists, ADR-0004), TLS ≥1.2, retry 5xx-only, `Get`/`Post`, `Mock` for tests. `APIVersion()` (the
  `/system` + `/sloprovisioning` prefix) is auto-detected from `GET /univmax/restapi/version`
  when `apiVersion` is unset (ADR-0015); `/performance` is unversioned.
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
- **Volume (LUN) collectors are opt-in** — one series set per device.
  `volume.go` (`collection.volumeMetrics`): perf, batched — one
  `POST /performance/Volume/metrics` per ≤10 storage groups (`storageGroups`
  comma-list), the only batched per-object path Unisphere has.
  `volume_inventory.go` (`collection.volumeInventory`): capacity + identity
  (WWN/identifier) via sloprovisioning volume list per SG (first page only,
  truncation logged) + one GET per volume.
- **FPIN** (`fpin.go`): Unisphere 10.5 `GET /univmax/rest/v1/systems/{id}/fpins` (Enhanced
  Endpoints root `/univmax/rest`, not `restapi`) → `pmax_fpin_*` per port/`fpin_type`;
  silent below 10.5 or on 404 (ADR-0015).
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
  until live-validated (ADR-0009); keys must exist in the vendored 10.5 spec
  (`docs/swagger/14405-10.5.0.json`, ADR-0015) — `catalog_spec_test.go` enforces it in `make ci`.
- **Never log credentials**: no resty `SetDebug`; `--trace` is a body-only
  `OnAfterResponse` hook with a test asserting no leak.
- **Grafana dashboards hardcode metric names** (`grafana/dashboards/*.json`): renaming
  a catalog metric (e.g. after ADR-0009 live validation) must update the dashboard
  queries in the same commit — the spec test fails `make ci` on any dashboard
  `pmax_*` name the exporter no longer emits.
- Default collection interval 5m = diagnostic granularity; don't "optimize" it down.
- No inline semgrep/`nolint` suppressions — restructure instead.

## Testing

`go test -race ./...` — mock-driven (`pmaxclient.Mock`); collector assertions go through
**both** the Prometheus registry gather and the OTLP `ManualReader`. The client is tested
against `httptest.NewTLSServer` (auth header, 5xx-vs-4xx retry, trace no-leak).

Gotcha: `Mock.SetPostFunc` is consulted **before** static `SetPostJSON` registrations —
a postFunc must return `(_, false)` for paths it doesn't own (e.g. keys POSTs), or it
silently swallows them and the test fails with a confusing parse error.

## CI/CD

`ci.yml` (make ci + SBOM + semgrep), `release.yml` (GoReleaser + GHCR multi-arch image on
`v*` tags), `docs.yml` (mkdocs → Pages) — all three call `fjacquet/ci` reusable
workflows at `@v1`. Third-party actions SHA-pinned with `# vX.Y.Z` comments;
`persist-credentials: false` everywhere; release builds with `cache: false`.
