# Swagger Validation Report — Phase 1

**Date:** 2026-06-14
**Branch:** `feat/swagger-catalog-validation`
**Canonical spec:** `docs/swagger/12315-10.4.0.json` (PowerMax 10.4) · skew vs `openapi-9.2.json` (9.2)
**Validator:** `internal/pmax/catalog_spec_test.go` + `catalog_spec_helpers_test.go` (report mode)

## Headline

**Zero drift across all four checks.** The provisional catalog (ADR-0009) is fully
consistent with the 10.4 OpenAPI spec — every exact-case perf key exists in the spec
enums, every dashboard query references a metric the exporter emits, every inventory field
exists in the spec response schema, and no catalog key is 10.4-only. Result is
deterministic over 5× `-count` runs.

## Results by check

| Check | Scope | Result |
|-------|-------|--------|
| 1. Perf keys ⊆ spec enum | 9 categories, all `MetricDef.Key` vs `{cat}Param.metrics` enum (10.4) | **0 drift** — every key present |
| 2. Dashboard refs ⊆ emitted | 2 dashboards (41 `pmax_*` refs) vs 80 emitted source literals | **0 drift** — every ref emitted |
| 3. Inventory fields ⊆ spec schema | 7 `volumeDetailResp` JSON tags vs sloprovisioning `volume` schema | **0 drift** — all 7 present |
| 4. 9.2 skew (non-failing) | catalog keys present in 10.4 but absent in 9.2 | **0 skew** — all keys in both versions |

## What this means

- The keys cross-checked against `kckecheng/powermax_exporter` and `PyU4V` during the
  original catalog build hold up against Dell's own 10.4 spec. The ADR-0009 "provisional
  until live-validated" caveat is now substantially de-risked — short of a live array, the
  vendor spec is the authority, and the catalog matches it.
- No catalog renames are needed, so **Phase 2 (fixes) is empty** — there is nothing to
  reconcile and no dashboard query to update.
- The catalog uses Unisphere-native keys that are stable across 9.2→10.4, so the exporter
  is safe against both Unisphere generations for the metrics it queries.

## One bug found and fixed during Phase 1

`specResponseProps` matched the *first* path ending `/volume/{volumeId}`, but **four**
paths share that suffix (sloprovisioning, mainframe, replication, vvol) resolving to
different schemas. Go's random map iteration made the inventory check flaky (~50% false
drift). Fixed in commit `3022e53`: the suffix is now fully qualified
(`/sloprovisioning/symmetrix/{symmetrixId}/volume/{volumeId}`) and `specResponseProps`
now `t.Fatalf`s on an ambiguous (>1) match rather than silently picking one — so a
too-loose suffix fails deterministically instead of flaking.

## Carried into Phase 3 (gate)

- `emittedNames` scans whole source files including comments, so a `pmax_*` name appearing
  only in a comment would be treated as "emitted" and could mask a real dashboard drift
  (noted by code review on Task 3). Acceptable in report mode; worth tightening to
  string-literal matching when Check 2 becomes a hard CI gate.
- The `specExceptions` allowlist will ship empty — there are currently no intentional
  spec deviations to record.

## Next step

Because drift is zero, Phase 2 is a no-op. Recommend proceeding straight to **Phase 3**:
flip `reportMode` to `false`, add the (empty) `specExceptions` scaffold, prove the gate
bites with an injected bad key, and wire it into `make ci`.
