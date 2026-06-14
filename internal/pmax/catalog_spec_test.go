package pmax

import (
	"path/filepath"
	"testing"
)

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
