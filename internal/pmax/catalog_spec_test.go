package pmax

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reportMode=false makes these checks hard CI gates (Errorf). Set to true
// temporarily to report-only — e.g. while triaging before adding a
// specExceptions entry.
const reportMode = false

// specExceptions are catalog keys intentionally kept despite absence from the
// canonical spec enum, because the live array accepts them and the spec is incomplete
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
	spec := specMetrics(t, specPath)
	for _, cat := range PerfCategories() {
		set, ok := spec[cat.Category]
		if !ok {
			reportf(t, "category %q has no /performance/%s/metrics enum in canonical spec",
				cat.Category, cat.Category)
			continue
		}
		for _, m := range cat.Metrics {
			if specExceptions[cat.Category+"/"+m.Key] != "" {
				continue
			}
			if !set[m.Key] {
				reportf(t, "category %q: key %q (-> %s) not in canonical spec enum",
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
	props := specResponseProps(t, specPath, "/sloprovisioning/symmetrix/{symmetrixId}/volume/{volumeId}")
	// JSON tags decoded by volumeDetailResp in volume_inventory.go.
	fields := []string{
		"volumeId", "cap_gb", "allocated_percent",
		"wwn", "volume_identifier", "type", "storageGroupId",
	}
	for _, f := range fields {
		if !props[f] {
			reportf(t, "volume detail field %q not in canonical volume schema", f)
		}
	}
}

func TestArrayInfoFieldsInSpec(t *testing.T) {
	props := specResponseProps(t, specPath, "/system/symmetrix/{symmetrixId}")
	// JSON tags decoded by symmetrixResp in arrayinfo.go. `ucode` is the
	// pre-10.x alias and absent from 10.x specs by design.
	fields := []string{"symmetrixId", "model", "microcode", "device_count", "disk_count", "cache_size_mb"}
	for _, f := range fields {
		if !props[f] {
			reportf(t, "array info field %q not in canonical systemSymmetrix schema", f)
		}
	}
}

func TestVolumePerfKeysInSpec(t *testing.T) {
	spec := specMetrics(t, specPath)
	set, ok := spec["Volume"]
	if !ok {
		t.Fatal("no /performance/Volume/metrics enum in canonical spec")
	}
	for _, m := range volumeMetricDefs {
		if specExceptions["Volume/"+m.Key] != "" {
			continue
		}
		if !set[m.Key] {
			reportf(t, "volume perf key %q (-> %s) not in canonical spec enum", m.Key, m.Name)
		}
	}
}

func TestCatalogSkew92(t *testing.T) {
	sCanon := specMetrics(t, specPath)
	s92 := specMetrics(t, spec92Path)
	for _, cat := range PerfCategories() {
		set92, in92 := s92[cat.Category]
		if !in92 {
			continue // category absent in 9.2 — not a per-key skew signal
		}
		for _, m := range cat.Metrics {
			if sCanon[cat.Category][m.Key] && !set92[m.Key] {
				t.Logf("SKEW: %s key %q present in canonical spec but absent in 9.2",
					cat.Category, m.Key)
			}
		}
	}
}

func TestFPINInSpec(t *testing.T) {
	raw, err := os.ReadFile(enhancedSpecPath)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	if _, ok := doc.Paths["/v1/systems/{id}/fpins"]["get"]; !ok {
		reportf(t, "GET /v1/systems/{id}/fpins not in enhanced spec")
	}
	lower := func(schema string) map[string]bool {
		m := map[string]bool{}
		for k := range doc.Components.Schemas[schema].Properties {
			m[strings.ToLower(k)] = true
		}
		return m
	}
	fpin := lower("Fpin")
	for _, f := range []string{"director_port", "fpin_type", "detected_time", "director"} {
		if !fpin[f] {
			reportf(t, "Fpin schema missing field %q", f)
		}
	}
	if !lower("pagingInfoType")["resume_token"] {
		reportf(t, "pagingInfoType missing resume_token")
	}
}
