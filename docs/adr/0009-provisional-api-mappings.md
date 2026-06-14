# Provisional API mappings & live validation

## Status
Accepted (mappings provisional until validated against a live Unisphere). Partially
addressed by [ADR-0010](0010-spec-validation-ci-gate.md): the catalog is now CI-validated
against the vendored Unisphere OpenAPI spec, which confirms a key *exists* in the API.
A live-array run (eased by the Windows trace binary, [ADR-0011](0011-windows-binary-for-trace-capture.md))
is still required to confirm *this array reports it*; live-only deviations go in the
`specExceptions` allowlist.

## Context
This exporter was built against documentation and reference implementations
(`dell/pmaxperfpy`, PyU4V, `dell/gopowermax`, `kckecheng/powermax_exporter`), not a live
array. Two classes of risk follow:

1. **Performance metric keys** (`catalog.go`) must match Unisphere's exact names —
   a wrong key in a `metrics` POST fails the whole query with a 400 (surfacing as
   `pmax_collector_up{collector="perf_…"} 0`). Keys like `HostMBs`, `PercentCacheWP`,
   `MBSentAndReceived`, `AllocatedCapacity` are cross-checked against PyU4V/pmaxperfpy
   but unconfirmed on-array.
2. **Inventory payload shapes** (`symmetrixResp`, `srpResp`) vary across Unisphere
   versions (`ucode` vs `microcode`, `srp_capacity` block layout). All such fields are
   pointers (or tolerant `any` decoding in the perf engine): an absent or unparseable
   field yields an **absent sample, never a zero** — a fake 0 on a capacity metric
   silently corrupts dashboards.

## Decision
- Keep every provisional shape localized to its collector file and marked provisional in
  comments.
- Validate with the live-array workflow:
  `pmax_exporter --config real.yaml --once --debug --trace 2>trace.log | sort > samples.txt`,
  then diff `samples.txt` against `docs/metrics.md` and fix names/shapes from the traced
  bodies. Update this ADR when validation happens.
- "Current" value of any time series = the **newest point by timestamp**, regardless of
  response order.

## Consequences
First contact with a real Unisphere may flag some perf categories down; the failure mode
is loud (collector_up=0 + traced 400 body naming the bad key), not silent wrong data.
The catalog fix is a one-line key rename.
