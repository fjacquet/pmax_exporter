# Spec: Swagger-driven catalog & dashboard validation

**Date:** 2026-06-14
**Status:** Approved (brainstorming) — ready for implementation plan
**Related:** ADR-0009 (catalog provisional until live-validated), ADR-0006 (label-key
consistency), CLAUDE.md "Grafana dashboards hardcode metric names" rule.

## Problem

The performance catalog (`internal/pmax/catalog.go`) is provisional (ADR-0009): every
`MetricDef.Key` is an exact-case Unisphere metric name, and one wrong key 400s the whole
category (`pmax_collector_up{collector="perf_…"} 0`). Until now the only way to validate
was a live array. We now have authoritative Unisphere OpenAPI specs checked into
`docs/swagger/`, and — critically — they **enumerate the exact-case metric names per
category** inside each `{cat}Param.metrics` request-body schema description. This lets us
validate the catalog against ground truth short of a live array, and lock that validation
into CI so the catalog and Grafana dashboards can't silently drift.

## Spec inventory (`docs/swagger/`)

Files are minified single-line JSON (note: `wc -l` reports 0 — they are *not* empty).

| File | Title | Version | Paths | Perf paths | Role |
|------|-------|---------|-------|-----------|------|
| `12315-10.4.0.json` | PowerMax | 10.4 | 470 | 137 | **Canonical** |
| `12316-10.4.0.json` | PowerMax – Enhanced Endpoints | 10.4 | 13 | 0 | Not used (no perf) |
| `openapi-9.2.json` | Unisphere for PowerMax | 9.2 | 330 | 124 | Skew cross-check |

**Decisions:** canonical spec = 10.4 (`12315`), with 9.2 cross-checked for version skew
(non-failing). Deliverable = report first, then fix. Scope = full (perf keys + categories
+ endpoints + inventory fields + dashboards).

## Key spec facts (verified)

- Each `/performance/{Cat}/metrics` POST `requestBody` `$ref`s a `{cat}Param` schema whose
  `metrics` property `description` lists valid names as `* **MetricName** - human text.`
- Param-schema casing is **inconsistent** (`arrayParam` lowercase, `feDirectorParam`
  camelCase). The validator MUST resolve the schema by following the metrics-POST
  requestBody `$ref`, never by mangling the category name.
- Enum extraction regex: `\*\s+\*\*([A-Za-z0-9_]+)\*\*`.
- All 9 catalog categories resolve, enum sizes 10–184: Array(135), FEDirector(105),
  BEDirector(31), FEPort(33), BEPort(10), CachePartition(47), RDFDirector(34),
  StorageGroup(184), SRP(56).

## Architecture

One Go test file, `internal/pmax/catalog_spec_test.go`, riding `go test ./...` → `make ci`.
Spec path relative to the package dir: `../../docs/swagger/`.

### Components

1. **`specMetrics(specPath) map[string]map[string]bool`** — parse the spec once; for each
   `/performance/{Cat}/metrics` path, follow requestBody `$ref` to the param schema, regex
   the `metrics` description into a set. Reused for 10.4 and 9.2.

2. **Check 1 — perf keys ⊆ spec (FAILING, the load-bearing check).** Iterate
   `PerfCategories()`; assert each `MetricDef.Key` ∈ the 10.4 set for `Category`. Two-level
   categories (FEPort/BEPort) have their own `fePortParam`/`bePortParam` and validate
   directly.

3. **Check 2 — dashboard refs ⊆ emitted (FAILING).** Glob `grafana/dashboards/*.json`,
   regex `pmax_[a-z0-9_]+` from panel `expr` strings. The **emitted set** is the real
   Prometheus registry family names gathered from a comprehensive mock collect (reuse the
   existing mock-collect test infra) — covers catalog `.Name` plus inventory, volume, and
   meta metrics (`pmax_collector_up`, scrape stats from `collector.go`). No hand-maintained
   list.

4. **Check 3 — inventory fields ⊆ spec response schema (FAILING).** Pull JSON tags off the
   `volume_inventory.go` structs; resolve the sloprovisioning volume GET response schema in
   the 10.4 spec; assert each tag exists as a property.

5. **Skew warning + exceptions.** Load 9.2 too; `t.Logf` (non-failing) any key present in
   10.4 but not 9.2. A `specExceptions` allowlist (category→key→justification) covers
   documented cases where the spec is wrong and the live array is right (ADR-0009); each
   entry carries an inline justification. Keeps the gate green for intentional deviations.

## Sequencing

This is how "report first, then fix" lands:

- **Phase 1 — report mode.** Build components 1–4 emitting `t.Log` only (never fail). One
  run = the discrepancy report. **User reviews.**
- **Phase 2 — fix.** Correct `catalog.go` keys, dashboard queries, and inventory tags per
  report. Each change gated by user approval. A catalog metric rename and its dashboard
  query update stay in the **same commit** (CLAUDE.md rule — nothing else catches the drift).
- **Phase 3 — gate.** Flip checks 1–4 to hard failures (with `specExceptions`), confirm
  `make ci` green, update the ADR-0009 / memory note that the catalog is now spec-validated.

## Out of scope

- No CI parsing of the 9.2 spec beyond the skew warning.
- No Windows build (release targets stay linux/darwin).
- No change to the perf engine, request envelope, or collection model — names only.
- The Enhanced-Endpoints spec (`12316`) is not consumed (no perf paths).

## Testing

The validator *is* test code. Phase 1 must run clean (logs only). Phase 3 must fail loudly
on an injected bad key and pass on the corrected catalog. Existing `go test -race ./...`
through both the Prometheus registry gather and the OTLP `ManualReader` stays green.
