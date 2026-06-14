# Opt-in volume (LUN) collectors & their cost model

## Status
Accepted. Retroactively records the decision shipped with the LUN deep-dive
(`internal/pmax/volume.go`, `volume_inventory.go`).

## Context
Per-volume (LUN) metrics are the highest-cardinality, highest-cost data PowerMax exposes:
an array can hold tens of thousands of devices. Unlike directors/ports/SGs, most volume
data cannot be fetched array-wide in one call — it is per-object. Collecting it on every
array, every cycle, by default would explode both Unisphere API load and Prometheus
series count, hurting the operators who don't need LUN granularity.

## Decision
- Volume collectors are **opt-in**, gated by config (`collection.volumeMetrics`,
  `collection.volumeInventory`) and scoped to selected storage groups — off by default.
- **Perf path is batched** where Unisphere allows it: one
  `POST /performance/Volume/metrics` per ≤10 storage groups (`volumeChunkSize`, comma-list
  `storageGroups`). This is the *only* batched per-object performance path Unisphere has;
  use it rather than per-volume perf POSTs.
- **Inventory path is N+1 by necessity**: list volumes per SG (first page only — truncation
  is logged loudly, ADR-0009 "loud, not silent"), then one
  `GET …/sloprovisioning/.../volume/{id}` per volume for capacity + identity (WWN,
  identifier). Bounded by the SG scope and `maxConcurrent`.
- Per-volume detail failures degrade gracefully (warn + skip the volume); only an
  all-failed batch errors the collector.

## Consequences
- Default deployments pay nothing for LUN data; operators who want it opt in and accept the
  N+1 inventory cost, which they bound by scoping storage groups.
- The inventory list reading only the first page means very large SGs are under-reported —
  surfaced via a truncation warning, mitigated by tighter SG scoping, never a silent
  partial.
- Volume perf keys (`volumeMetricDefs`) live outside `PerfCategories()`, so they are
  validated separately against the spec's `Volume` enum (ADR-0010, check 1).
