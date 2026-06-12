# Label-key consistency invariant

## Status
Accepted.

## Context
Prometheus rejects (and dashboards silently mis-join) metric families whose series carry
different label-key sets. With one process serving many Unisphere instances and arrays,
every emission path must agree on the keys per family.

## Decision
A metric name carries **one** label-key set across all its series, in a fixed order:

- `server` (Unisphere instance, stamped by the collection loop) is first on every metric.
- `array` (symmetrix ID) is on every array-scoped metric.
- Object categories append exactly one object label: `director`, `storage_group`, or `srp`.

Each family is emitted by exactly one collector path (the generic perf engine derives
labels from the category definition), so divergence cannot arise structurally; a test
(`TestPerfLabelKeysConsistentAcrossSeries`) enforces it anyway, and the Prometheus
collector skips inconsistent series rather than panicking as a last line of defense.

## Consequences
`sum by (array)` and friends work across every family. Adding enrichment labels later
(e.g. array model on perf metrics) requires adding them to *every* series of the family,
empty when unresolved — never conditionally.
