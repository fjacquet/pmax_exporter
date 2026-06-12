# Snapshot collection model

## Status
Accepted.

## Context
Scrape-time collection couples backend API load to the number of Prometheus scrapers and
the OTLP push cadence. Unisphere performance queries are not free — object-level
categories cost one POST per object — and diagnostic data only changes every 5 minutes.

## Decision
A single background **collection loop** polls every configured Unisphere instance on
`collection.interval` (default 5m, matching diagnostic granularity) and publishes an
**immutable Snapshot** into a `SnapshotStore` (RWMutex pointer-swap). Both export paths —
the unchecked Prometheus collector and the OTLP observable gauges — read the latest
snapshot rather than fetching on scrape.

```
loop → discover arrays → run collectors → immutable Snapshot → SnapshotStore.Swap()
                                                ├── PromCollector (/metrics)
                                                └── OTLPExporter (push)
```

Per-instance failures degrade gracefully: `pmax_up{server}` and
`pmax_collector_up{server,collector}` gauges mark what failed; sibling collectors and
instances keep publishing.

## Consequences
Unisphere sees a fixed query load regardless of scraper count. Scrapes are O(snapshot)
memory reads. A scrape during a backend outage serves the last good data with `pmax_up=0`
flagging staleness, plus `pmax_array_perf_timestamp_seconds` for data-age alerting.
