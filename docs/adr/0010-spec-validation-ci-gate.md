# Catalog & dashboard validation against vendored OpenAPI spec

## Status
Accepted. Builds on [ADR-0009](0009-provisional-api-mappings.md) (does not supersede it —
live-array validation remains the final step).

## Context
ADR-0009 left the perf catalog "provisional until a live Unisphere confirms it", with the
only validation path being a manual on-array trace. That is a weak guarantee: a wrong
exact-case key (`HostMBs` vs `HostMbs`) 400s a whole category, and a renamed metric
silently desyncs the hardcoded Grafana dashboard queries — nothing in CI caught either.

Dell publishes Unisphere OpenAPI specs that **enumerate the exact-case metric names per
performance category** (inside each `/performance/{Cat}/metrics` POST `requestBody` param
schema) and the inventory response schemas. That is an authoritative source short of a
live array — and it can be diffed in CI.

## Decision
- **Vendor the specs** under `docs/swagger/` (`12315-10.4.0.json` = PowerMax 10.4,
  canonical; `openapi-9.2.json` = 9.2, skew cross-check). 10.4 is the source of truth;
  9.2 is informational.
- Add a CI-gated validator (`internal/pmax/catalog_spec_test.go` + helpers) with four
  checks, hard-failing in `make ci`:
  1. every catalog `MetricDef.Key` (including `volumeMetricDefs`) exists in the spec's
     per-category `metrics` enum;
  2. every `pmax_*` referenced in `grafana/dashboards/*.json` is actually emitted —
     emitted set extracted from Go **string literals via `go/parser`** (not raw text, so
     comments and identifiers don't create false "emitted" names);
  3. volume-inventory JSON tags exist in the spec's `volume` response schema;
  4. a non-failing 9.2-vs-10.4 skew warning.
- Resolve each category's param schema by **following the metrics-POST `$ref`**, never by
  name-mangling — Unisphere's schema-name casing is inconsistent (`arrayParam` vs
  `feDirectorParam`).
- Keep a `specExceptions` allowlist (currently empty) for keys intentionally retained
  despite spec absence, when a live array accepts them and the spec is incomplete (the
  ADR-0009 escape hatch); each entry carries a required justification.

## Consequences
- The catalog and dashboards can no longer drift undetected — a bad key or an orphan
  dashboard query fails CI with the offending name. First validation found **zero drift**:
  the provisional catalog already matched the 10.4 spec exactly.
- Spec-validation de-risks but does not replace ADR-0009's live run: the spec proves a key
  *exists*, not that *this array reports it*. Live discrepancies go in `specExceptions`.
- Vendoring third-party specs means **scrubbing example secrets** baked into them: Dell's
  9.2 spec carried example AWS keys/secrets that tripped GitHub push protection; they were
  redacted to placeholders (the validator never reads those fields). Re-vendoring an
  updated spec must repeat the scrub.
- Specs are minified single-line JSON (`wc -l` reports 0 — not empty); parse, don't
  line-read. The 10.4 spec is ~3 MB and decoded a few times per test run (acceptable;
  cache with `sync.Once` if CI time regresses).
