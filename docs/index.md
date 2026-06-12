# pmax_exporter

Prometheus + OTLP exporter for **Dell PowerMax**, talking to Unisphere for PowerMax's
REST API. One exporter process monitors any number of Unisphere instances and arrays.

## Highlights

- **Snapshot model** — a background loop polls Unisphere on the diagnostic 5-minute
  cadence; scrapes read an immutable in-memory snapshot (backend load is independent of
  scraper count).
- **Dual export** — Prometheus `/metrics` and optional OTLP gRPC push from the same
  snapshot.
- **Array, FE/BE/RDF director, storage group, and SRP performance**, plus SRP capacity
  and array inventory. Full list: [Metrics reference](metrics.md).
- **Multi-array, multi-instance** — every metric carries `server` (Unisphere) and
  `array` (symmetrix ID) labels; optional per-instance array allowlist.
- **Operational niceties** — config hot reload (SIGHUP + file watch), `${ENV}` secrets
  interpolation + `passwordFile`, `.env` quickstart, `/health` endpoint,
  `--once --debug` sample dump, token-safe `--trace`.

## How it works

```
Unisphere ×N ──(REST, Basic/TLS)── collection loop ── Snapshot ──┬── /metrics (Prometheus)
                                                                 └── OTLP push (optional)
```

Start at [Quick start](getting-started/quickstart.md), then see the
[Architecture decisions](adr/index.md) for the why behind the design.
