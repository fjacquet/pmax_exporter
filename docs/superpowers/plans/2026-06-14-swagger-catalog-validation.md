# Swagger Catalog & Dashboard Validation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A checked-in Go test that validates the perf catalog keys, Grafana dashboard metric refs, and volume-inventory fields against the vendored Unisphere 10.4 OpenAPI spec, runs report-first, then becomes a `make ci` gate.

**Architecture:** One test file `internal/pmax/catalog_spec_test.go` plus a small helper file `internal/pmax/catalog_spec_helpers_test.go`. A spec loader parses `docs/swagger/12315-10.4.0.json` into `category → {exact-case metric names}` by following each `/performance/{Cat}/metrics` POST requestBody `$ref`. Three checks (perf keys, dashboard refs, inventory fields) plus a non-failing 9.2 skew warning. Built in report mode first (`t.Log`), flipped to failing (`t.Error`) after the catalog/dashboards are reconciled.

**Tech Stack:** Go 1.26, `encoding/json` (generic `map[string]any` walk — no new deps), `regexp`, standard `testing`. Spec files already vendored under `docs/swagger/`.

---

## File Structure

- **Create** `internal/pmax/catalog_spec_helpers_test.go` — `specMetrics`, `specResponseProps`, `emittedNames`, `dashboardRefs` helpers + the spec-path constants. Pure parsing, independently unit-tested.
- **Create** `internal/pmax/catalog_spec_test.go` — the three checks + skew warning + `specExceptions` allowlist.
- **Modify (Phase 2, report-driven)** `internal/pmax/catalog.go` — correct any `MetricDef.Key` not in the spec enum.
- **Modify (Phase 2, report-driven)** `grafana/dashboards/pmax-overview.json`, `grafana/dashboards/pmax-lun-deep-dive.json` — fix any metric ref not in the emitted set, in the *same commit* as the corresponding catalog rename (CLAUDE.md rule).
- **Modify (Phase 3)** `Makefile` only if the gate needs an explicit target — otherwise `make ci`'s existing `go test -race ./...` already runs it.

**Spec facts already verified (do not re-derive):**
- Param-schema casing is inconsistent (`arrayParam`, `feDirectorParam`) — resolve via the metrics-POST `$ref`, never by name-mangling.
- Enum regex: `` \*\s+\*\*([A-Za-z0-9_]+)\*\* ``. Counts: Array 135, FEDirector 105, BEDirector 31, FEPort 33, BEPort 10, CachePartition 47, RDFDirector 34, StorageGroup 184, SRP 56.
- Volume detail GET path ends `/volume/{volumeId}` (version-prefixed `/104/…`; match by suffix), response `$ref` = `#/components/schemas/volume`; all 7 inventory JSON tags (`volumeId`, `cap_gb`, `allocated_percent`, `wwn`, `volume_identifier`, `type`, `storageGroupId`) are present.
- Spec path relative to `internal/pmax/`: `../../docs/swagger/12315-10.4.0.json` (10.4) and `../../docs/swagger/openapi-9.2.json` (9.2).

---

## PHASE 1 — Report-mode tooling (checks log, never fail)

### Task 1: Spec loader + its unit test

**Files:**
- Create: `internal/pmax/catalog_spec_helpers_test.go`
- Test: same file (`TestSpecMetricsLoads`)

- [ ] **Step 1: Write the failing test**

```go
package pmax

import "testing"

func TestSpecMetricsLoads(t *testing.T) {
	got := specMetrics(t, spec104Path)
	want := map[string]int{
		"Array": 135, "FEDirector": 105, "BEDirector": 31, "FEPort": 33,
		"BEPort": 10, "CachePartition": 47, "RDFDirector": 34,
		"StorageGroup": 184, "SRP": 56,
	}
	for cat, n := range want {
		if len(got[cat]) != n {
			t.Errorf("category %s: got %d metrics, want %d", cat, len(got[cat]), n)
		}
	}
	if !got["StorageGroup"]["HostIOs"] {
		t.Errorf("StorageGroup enum missing known key HostIOs")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/pmax/ -run TestSpecMetricsLoads -v`
Expected: FAIL — `undefined: specMetrics` / `undefined: spec104Path`.

- [ ] **Step 3: Implement the loader in the same file**

Add above the test:

```go
import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

const (
	spec104Path = "../../docs/swagger/12315-10.4.0.json"
	spec92Path  = "../../docs/swagger/openapi-9.2.json"
)

var enumRe = regexp.MustCompile(`\*\s+\*\*([A-Za-z0-9_]+)\*\*`)

// specMetrics returns category -> set of exact-case Unisphere metric names, read
// from each /performance/{Cat}/metrics POST requestBody $ref param schema. Casing
// of the param schema name is inconsistent, so we follow the $ref, never guess it.
func specMetrics(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec %s: %v", path, err)
	}
	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec %s: %v", path, err)
	}
	out := map[string]map[string]bool{}
	prefix, suffix := "/performance/", "/metrics"
	for p, methods := range doc.Paths {
		if len(p) <= len(prefix)+len(suffix) ||
			p[:len(prefix)] != prefix || p[len(p)-len(suffix):] != suffix {
			continue
		}
		cat := p[len(prefix) : len(p)-len(suffix)]
		if cat == "" || containsByte(cat, '/') { // skip help/{...} sub-paths
			continue
		}
		ref := metricsParamRef(methods["post"])
		if ref == "" {
			continue
		}
		desc := metricsEnumDesc(doc.Components.Schemas[ref])
		set := map[string]bool{}
		for _, m := range enumRe.FindAllStringSubmatch(desc, -1) {
			set[m[1]] = true
		}
		if len(set) > 0 {
			out[cat] = set
		}
	}
	return out
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

// metricsParamRef pulls the schema name out of post.requestBody.content.
// application/json.schema.$ref (e.g. "#/components/schemas/storageGroupParam").
func metricsParamRef(post json.RawMessage) string {
	var op struct {
		RequestBody struct {
			Content struct {
				JSON struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"application/json"`
			} `json:"content"`
		} `json:"requestBody"`
	}
	if json.Unmarshal(post, &op) != nil {
		return ""
	}
	ref := op.RequestBody.Content.JSON.Schema.Ref
	if i := lastSlash(ref); i >= 0 {
		return ref[i+1:]
	}
	return ""
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// metricsEnumDesc returns the `metrics` property description that carries the
// `* **Name** - text` enumeration (sometimes under items, sometimes direct).
func metricsEnumDesc(schema json.RawMessage) string {
	var s struct {
		Properties struct {
			Metrics struct {
				Description string `json:"description"`
				Items       struct {
					Description string `json:"description"`
				} `json:"items"`
			} `json:"metrics"`
		} `json:"properties"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return ""
	}
	if s.Properties.Metrics.Items.Description != "" {
		return s.Properties.Metrics.Items.Description
	}
	return s.Properties.Metrics.Description
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/pmax/ -run TestSpecMetricsLoads -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pmax/catalog_spec_helpers_test.go
git commit -m "test: OpenAPI spec metric-enum loader for catalog validation"
```

---

### Task 2: Check 1 — perf keys vs spec (report mode)

**Files:**
- Create: `internal/pmax/catalog_spec_test.go`

- [ ] **Step 1: Write the report-mode check**

```go
package pmax

import "testing"

// reportMode keeps the checks logging-only until the catalog/dashboards are
// reconciled (Phase 2). Flip to false in Phase 3 to make this a CI gate.
const reportMode = true

func reportf(t *testing.T, format string, a ...any) {
	t.Helper()
	if reportMode {
		t.Logf("DRIFT: "+format, a...)
	} else {
		t.Errorf(format, a...)
	}
}

func TestCatalogPerfKeysInSpec(t *testing.T) {
	spec := specMetrics(t, spec104Path)
	for _, cat := range PerfCategories() {
		set, ok := spec[cat.Category]
		if !ok {
			reportf(t, "category %q has no /performance/%s/metrics enum in 10.4 spec",
				cat.Category, cat.Category)
			continue
		}
		for _, m := range cat.Metrics {
			if !set[m.Key] {
				reportf(t, "category %q: key %q (-> %s) not in 10.4 spec enum",
					cat.Category, m.Key, m.Name)
			}
		}
	}
}
```

- [ ] **Step 2: Run and capture the report**

Run: `go test ./internal/pmax/ -run TestCatalogPerfKeysInSpec -v`
Expected: PASS (report mode never fails). Capture every `DRIFT:` line — these are the perf-key discrepancies for the Phase-1 report.

- [ ] **Step 3: Commit**

```bash
git add internal/pmax/catalog_spec_test.go
git commit -m "test: report perf catalog keys not present in 10.4 spec"
```

---

### Task 3: emitted-name scanner + Check 2 — dashboard refs (report mode)

**Files:**
- Modify: `internal/pmax/catalog_spec_helpers_test.go` (add `emittedNames`, `dashboardRefs`)
- Modify: `internal/pmax/catalog_spec_test.go` (add `TestDashboardRefsEmitted`)

- [ ] **Step 1: Add the helpers (append to `catalog_spec_helpers_test.go`)**

```go
import "path/filepath" // add to the import block

var pmaxNameRe = regexp.MustCompile(`pmax_[a-z0-9_]+`)

// emittedNames is every pmax_* metric the exporter can emit: the set of pmax_
// string literals in non-test Go source under internal/. A name only reaches a
// dashboard if some collector emits a Sample with that literal, so this is the
// authoritative, drift-proof emitted set (no hand-maintained list).
func emittedNames(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	matches, err := filepath.Glob("*.go") // internal/pmax; widen if collectors move
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range matches {
		if len(f) > 8 && f[len(f)-8:] == "_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, n := range pmaxNameRe.FindAllString(string(b), -1) {
			set[n] = true
		}
	}
	return set
}

// dashboardRefs returns every pmax_* metric referenced across the dashboard JSONs.
func dashboardRefs(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("../../grafana/dashboards/*.json")
	if err != nil {
		t.Fatalf("glob dashboards: %v", err)
	}
	out := map[string][]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		seen := map[string]bool{}
		for _, n := range pmaxNameRe.FindAllString(string(b), -1) {
			if !seen[n] {
				seen[n] = true
				out[f] = append(out[f], n)
			}
		}
	}
	return out
}
```

> NOTE on `emittedNames`: collectors live in `internal/pmax/` (same dir as the test), so `*.go` glob suffices. If a future collector emits names from another package, widen the glob then.

- [ ] **Step 2: Add the check to `catalog_spec_test.go`**

```go
func TestDashboardRefsEmitted(t *testing.T) {
	emitted := emittedNames(t)
	for file, refs := range dashboardRefs(t) {
		for _, ref := range refs {
			if !emitted[ref] {
				reportf(t, "dashboard %s references %q which no collector emits",
					filepath.Base(file), ref)
			}
		}
	}
}
```

Add `"path/filepath"` to the test file's imports.

- [ ] **Step 3: Run and capture**

Run: `go test ./internal/pmax/ -run 'TestDashboardRefsEmitted' -v`
Expected: PASS. Capture `DRIFT:` lines — dashboard refs with no emitter.

- [ ] **Step 4: Sanity-check the emitted scanner**

Run: `go test ./internal/pmax/ -run TestSpecMetricsLoads -v` is unaffected; then eyeball that `emittedNames` picked up known names by adding a temporary `t.Logf("%d emitted", len(emitted))` if a dashboard ref looks wrongly flagged. Remove the temp log before commit.

- [ ] **Step 5: Commit**

```bash
git add internal/pmax/catalog_spec_helpers_test.go internal/pmax/catalog_spec_test.go
git commit -m "test: report dashboard metric refs not emitted by any collector"
```

---

### Task 4: inventory fields vs spec response schema (report mode)

**Files:**
- Modify: `internal/pmax/catalog_spec_helpers_test.go` (add `specResponseProps`)
- Modify: `internal/pmax/catalog_spec_test.go` (add `TestInventoryFieldsInSpec`)

- [ ] **Step 1: Add `specResponseProps` helper**

```go
// specResponseProps returns the property names of the 200 response schema for the
// first path whose key ends with pathSuffix (paths are version-prefixed, e.g.
// /104/sloprovisioning/.../volume/{volumeId}).
func specResponseProps(t *testing.T, path, pathSuffix string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var doc struct {
		Paths map[string]struct {
			Get struct {
				Responses struct {
					OK struct {
						Content struct {
							JSON struct {
								Schema struct {
									Ref string `json:"$ref"`
								} `json:"schema"`
							} `json:"application/json"`
						} `json:"content"`
					} `json:"200"`
				} `json:"responses"`
			} `json:"get"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	for p, item := range doc.Paths {
		if len(p) >= len(pathSuffix) && p[len(p)-len(pathSuffix):] == pathSuffix {
			name := item.Get.Responses.OK.Content.JSON.Schema.Ref
			if i := lastSlash(name); i >= 0 {
				name = name[i+1:]
			}
			props := map[string]bool{}
			for k := range doc.Components.Schemas[name].Properties {
				props[k] = true
			}
			return props
		}
	}
	t.Fatalf("no path ending %q in %s", pathSuffix, path)
	return nil
}
```

- [ ] **Step 2: Add the check to `catalog_spec_test.go`**

```go
func TestInventoryFieldsInSpec(t *testing.T) {
	props := specResponseProps(t, spec104Path, "/volume/{volumeId}")
	// JSON tags decoded by volumeDetailResp in volume_inventory.go.
	fields := []string{
		"volumeId", "cap_gb", "allocated_percent",
		"wwn", "volume_identifier", "type", "storageGroupId",
	}
	for _, f := range fields {
		if !props[f] {
			reportf(t, "volume detail field %q not in 10.4 volume schema", f)
		}
	}
}
```

- [ ] **Step 3: Run and capture**

Run: `go test ./internal/pmax/ -run TestInventoryFieldsInSpec -v`
Expected: PASS, and per verified facts **no DRIFT lines** (all 7 fields resolve). If any appear, record them in the report.

- [ ] **Step 4: Commit**

```bash
git add internal/pmax/catalog_spec_helpers_test.go internal/pmax/catalog_spec_test.go
git commit -m "test: report volume inventory fields not in 10.4 spec schema"
```

---

### Task 5: 9.2 skew warning (non-failing, always)

**Files:**
- Modify: `internal/pmax/catalog_spec_test.go`

- [ ] **Step 1: Add the skew test**

```go
func TestCatalogSkew92(t *testing.T) {
	s104 := specMetrics(t, spec104Path)
	s92 := specMetrics(t, spec92Path)
	for _, cat := range PerfCategories() {
		set92, in92 := s92[cat.Category]
		if !in92 {
			continue // category absent in 9.2 — not a per-key skew signal
		}
		for _, m := range cat.Metrics {
			if s104[cat.Category][m.Key] && !set92[m.Key] {
				t.Logf("SKEW: %s key %q present in 10.4 but absent in 9.2",
					cat.Category, m.Key)
			}
		}
	}
}
```

- [ ] **Step 2: Run and capture**

Run: `go test ./internal/pmax/ -run TestCatalogSkew92 -v`
Expected: PASS. `SKEW:` lines feed the report's version-compatibility note.

- [ ] **Step 3: Commit**

```bash
git add internal/pmax/catalog_spec_test.go
git commit -m "test: warn on catalog keys present in 10.4 but absent in 9.2"
```

- [ ] **Step 4: Produce the Phase-1 report**

Run the full set and collect all `DRIFT:`/`SKEW:` lines:

```bash
go test ./internal/pmax/ -run 'Catalog|Dashboard|Inventory' -v 2>&1 | grep -E 'DRIFT|SKEW'
```

Write `docs/superpowers/specs/2026-06-14-swagger-validation-report.md` summarising: perf-key drift, dashboard drift, inventory drift (expected none), skew. **STOP — user reviews the report before Phase 2.**

---

## PHASE 2 — Fix (report-driven, gated per change)

> The exact edits depend on the Phase-1 report. Apply this procedure once per discrepancy.

### Task 6: Reconcile catalog keys, dashboards, inventory

**Files:** `internal/pmax/catalog.go`, `grafana/dashboards/*.json`, possibly `internal/pmax/volume_inventory.go`

- [ ] **Step 1 (per perf-key DRIFT):** Find the correct exact-case name in the spec enum.

```bash
go test ./internal/pmax/ -run TestSpecMetricsLoads -v   # loader works
# inspect the enum for a category:
python3 -c "import json,re; d=json.load(open('docs/swagger/12315-10.4.0.json')); \
import sys; print('\n'.join(sorted(re.findall(r'\*\s+\*\*([A-Za-z0-9_]+)\*\*', \
d['components']['schemas']['storageGroupParam']['properties']['metrics']['items']['description']))))"
```

- [ ] **Step 2:** Decide per discrepancy: (a) typo/casing → fix `MetricDef.Key` in `catalog.go`; (b) spec wrong / live array right (ADR-0009) → leave key, add a `specExceptions` entry (Task 7) with justification; (c) metric genuinely unavailable → remove the `MetricDef` and its dashboard panel.

- [ ] **Step 3 (if a `MetricDef.Name` changes):** Update the dashboard query referencing the old name **in the same commit**. Find it:

```bash
grep -rn 'pmax_old_name' grafana/dashboards/
```

- [ ] **Step 4:** After each fix, re-run the relevant check; the `DRIFT:` line for that item must disappear.

Run: `go test ./internal/pmax/ -run 'CatalogPerfKeys|DashboardRefs' -v 2>&1 | grep DRIFT`

- [ ] **Step 5:** Commit per logical fix (catalog rename + its dashboard edit together):

```bash
git add internal/pmax/catalog.go grafana/dashboards/<touched>.json
git commit -m "fix: correct <category> metric key <X> per 10.4 spec (+ dashboard)"
```

---

## PHASE 3 — Gate

### Task 7: Exceptions allowlist + flip to failing + wire into CI

**Files:** `internal/pmax/catalog_spec_test.go`, `docs/adr/` or memory note

- [ ] **Step 1: Add the exceptions allowlist** (above the checks)

```go
// specExceptions are catalog keys intentionally kept despite absence from the
// 10.4 spec enum, because the live array accepts them and the spec is incomplete
// (ADR-0009). Key: "Category/MetricKey". Value: justification (required).
var specExceptions = map[string]string{
	// "SRP/SomeKey": "validated live on uni01 2026-06-..; spec enum omits it",
}
```

- [ ] **Step 2: Honour exceptions in Check 1** — in `TestCatalogPerfKeysInSpec`, before `reportf`, skip allowlisted keys:

```go
if specExceptions[cat.Category+"/"+m.Key] != "" {
	continue
}
```

- [ ] **Step 3: Flip the gate**

Change `const reportMode = true` to `const reportMode = false` in `catalog_spec_test.go`.

- [ ] **Step 4: Run the full gate**

Run: `go test -race ./internal/pmax/ -run 'Catalog|Dashboard|Inventory' -v`
Expected: PASS (all real drift fixed; remaining intentional deviations allowlisted).

- [ ] **Step 5: Inject-a-bug verification** (prove the gate bites)

Temporarily change one `MetricDef.Key` in `catalog.go` to `"BogusKey"`, run:

Run: `go test ./internal/pmax/ -run TestCatalogPerfKeysInSpec`
Expected: FAIL naming `BogusKey`. Revert the change; re-run → PASS.

- [ ] **Step 6: Confirm `make ci` runs it**

Run: `make ci`
Expected: PASS — `go test -race ./...` already includes the new file; no Makefile change needed.

- [ ] **Step 7: Record validation status**

Update the memory note `pmax-exporter-status.md`: catalog is now spec-validated against 10.4; list any `specExceptions`. Commit:

```bash
git add internal/pmax/catalog_spec_test.go
git commit -m "test: make swagger catalog/dashboard validation a CI gate"
```

---

## Self-Review

- **Spec coverage:** Check 1 (Task 2) = perf keys ⊆ spec; Check 2 (Task 3) = dashboard refs ⊆ emitted; Check 3 (Task 4) = inventory fields ⊆ spec; skew (Task 5); report-first (Tasks 1–5 report mode + Task 5 step 4); fix (Task 6); gate + exceptions (Task 7). All spec sections mapped.
- **Deviation from spec doc:** Check 2's emitted set is derived by scanning `pmax_*` source literals (Task 3) rather than a mock-collect registry gather — same "no hand-maintained list" intent, more reliable given all names are static literals. Flagged to user at handoff.
- **Placeholder scan:** none — every code step is complete Go. Task 6 is deliberately a procedure (its edits depend on the Phase-1 report), with concrete commands.
- **Type consistency:** `specMetrics`, `specResponseProps`, `emittedNames`, `dashboardRefs`, `reportf`, `reportMode`, `specExceptions` used consistently across tasks; spec-path constants defined once in Task 1.
- **Categories→param resolution** uses the metrics-POST `$ref` (verified), not name-mangling.
