package pmax

import (
	"path/filepath"
	"testing"
)

// reportMode=false makes these checks hard CI gates (Errorf). Set to true
// temporarily to report-only — e.g. while triaging before adding a
// specExceptions entry.
const reportMode = false

// specExceptions are catalog keys intentionally kept despite absence from the
// 10.4 spec enum, because the live array accepts them and the spec is incomplete
// (ADR-0009). Key: "Category/MetricKey". Value: justification (required, non-empty).
var specExceptions = map[string]string{
	// none currently — the catalog is fully spec-consistent as of 2026-06-14.
}

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
			if specExceptions[cat.Category+"/"+m.Key] != "" {
				continue
			}
			if !set[m.Key] {
				reportf(t, "category %q: key %q (-> %s) not in 10.4 spec enum",
					cat.Category, m.Key, m.Name)
			}
		}
	}
}

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

func TestInventoryFieldsInSpec(t *testing.T) {
	props := specResponseProps(t, spec104Path, "/sloprovisioning/symmetrix/{symmetrixId}/volume/{volumeId}")
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

func TestVolumePerfKeysInSpec(t *testing.T) {
	spec := specMetrics(t, spec104Path)
	set, ok := spec["Volume"]
	if !ok {
		t.Fatal("no /performance/Volume/metrics enum in 10.4 spec")
	}
	for _, m := range volumeMetricDefs {
		if specExceptions["Volume/"+m.Key] != "" {
			continue
		}
		if !set[m.Key] {
			reportf(t, "volume perf key %q (-> %s) not in 10.4 spec enum", m.Key, m.Name)
		}
	}
}

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
