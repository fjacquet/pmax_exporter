# Hand-rolled `resty/v2` client (gopowermax not adopted)

## Status
Accepted.

## Context
The family rule: use the official vendor Go SDK if (1) available and (2) useful; otherwise
hand-roll a lean `resty/v2` client and record the failing criteria.

For PowerMax an official SDK **is available**: [`dell/gopowermax`](https://github.com/dell/gopowermax)
v2 (the library behind the CSI driver and `csm-metrics-powermax`). Evaluated against the
four usefulness criteria:

1. **Modern auth — pass.** Unisphere's current flow is HTTP Basic over TLS (no
   bearer/token endpoint exists); gopowermax implements it.
2. **Batched/efficient stats — fail.** The SDK's performance surface is CSI/CSM-scoped:
   `GetStorageGroupMetrics`, `GetVolumesMetrics(ByID)`, `GetFileSystemMetricsByID`, plus
   Array/StorageGroup keys. **No Array, FEDirector, BEDirector, RDFDirector, Port, SRP, or
   cache performance calls** — the heart of a general-purpose exporter (the same
   disqualifier that ruled out `goobjectscale` for `obs_exporter`).
3. **Models the objects + metrics we export — fail** for the performance layer (see 2);
   provisioning/system coverage alone doesn't justify the dependency.
4. **No regression — pass.** Light dependency tree, `go 1.26` floor.

Unlike `gopowerstore` there is **no generic raw-query escape hatch** to fill the gaps, so
"SDK + escape hatch" is not an option.

Decisively: the Unisphere performance REST API is uniform —
`POST /performance/{Category}/keys` to discover objects, then
`POST /performance/{Category}/metrics` with
`{symmetrixId, startDate, endDate, dataFormat: "Average", metrics: [...]}` plus a
category-specific id field — so **one generic engine** (`internal/pmax/perf.go`) covers
every category, present and future, with a curated catalog per category.

## Decision
Hand-roll a lean `github.com/go-resty/resty/v2` client (`internal/pmaxclient`): Basic auth
per request, TLS ≥ 1.2, 5xx-only retry, `Get`/`Post` + JSON decoding, and a `Client`
interface with an in-memory `Mock` for tests. Drive all performance categories through the
single generic `Perf` collector.

## Consequences
Adding a performance category is a catalog entry, not an SDK feature request. The
reference repos remain useful as cross-checks: `dell/pmaxperfpy` and PyU4V for endpoint
semantics and metric names, `csm-metrics-powermax` for Dell's own interpretation of
SG/volume metrics (see ADR-0009 for the validation workflow).
