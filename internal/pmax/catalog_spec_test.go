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
