package pmax

import (
	"context"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

var testArrays = []ArrayWindow{{ID: "000297900046", Last: 1700000300000}}

func arrayCat() PerfCategory {
	return PerfCategory{
		Category: "Array",
		Metrics: []MetricDef{
			{Key: "HostIOs", Name: "pmax_array_host_iops"},
			{Key: "HostMBs", Name: "pmax_array_host_megabytes_per_second"},
		},
	}
}

func findSample(t *testing.T, samples []Sample, name string) Sample {
	t.Helper()
	for _, s := range samples {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("sample %s not found in %d samples", name, len(samples))
	return Sample{}
}

func TestPerfArrayPicksNewestDatapoint(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/Array/metrics", `{
	  "resultList": {"result": [
	    {"HostIOs": 100.0, "HostMBs": 10.0, "timestamp": 1700000000000},
	    {"HostIOs": 250.5, "HostMBs": 25.0, "timestamp": 1700000300000}
	  ]}
	}`)
	p := Perf{Cat: arrayCat(), MaxConcurrent: 2}
	samples, err := p.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	s := findSample(t, samples, "pmax_array_host_iops")
	if s.Value != 250.5 {
		t.Fatalf("HostIOs = %v, want newest datapoint 250.5", s.Value)
	}
	if s.LabelValue("array") != "000297900046" {
		t.Fatalf("array label = %q", s.LabelValue("array"))
	}
}

func TestPerfAbsentMetricIsAbsentNotZero(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	// HostMBs missing, and a junk string for HostIOs that still parses.
	m.SetPostJSON("/univmax/restapi/performance/Array/metrics", `{
	  "resultList": {"result": [{"HostIOs": " 42.5 ", "timestamp": 1700000300000}]}
	}`)
	p := Perf{Cat: arrayCat()}
	samples, err := p.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if s := findSample(t, samples, "pmax_array_host_iops"); s.Value != 42.5 {
		t.Fatalf("string-typed HostIOs = %v, want 42.5", s.Value)
	}
	for _, s := range samples {
		if s.Name == "pmax_array_host_megabytes_per_second" {
			t.Fatal("absent metric must not be emitted as zero")
		}
	}
}

func TestPerfEmptyResultEmitsNothing(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/Array/metrics", `{"resultList": {"result": []}}`)
	p := Perf{Cat: arrayCat()}
	samples, err := p.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("samples = %d, want 0 for empty result", len(samples))
	}
}

func feCat() PerfCategory {
	return PerfCategory{
		Category: "FEDirector", IDField: "directorId", ObjLabel: "director",
		Metrics: []MetricDef{{Key: "PercentBusy", Name: "pmax_fe_director_busy_percent"}},
	}
}

func TestPerfObjectCategoryDiscoversAndLabels(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/FEDirector/keys", `{
	  "feDirectorInfo": [
	    {"directorId": "FA-1D", "firstAvailableDate": 1690000000000, "lastAvailableDate": 1700000300000},
	    {"directorId": "FA-2D", "firstAvailableDate": 1690000000000, "lastAvailableDate": 1700000300000}
	  ]
	}`)
	m.SetPostFunc(func(path string, body any) (string, bool) {
		if path != "/univmax/restapi/performance/FEDirector/metrics" {
			return "", false
		}
		b := body.(map[string]any)
		if b["startDate"] != int64(1700000300000) || b["endDate"] != int64(1700000300000) {
			t.Errorf("metrics query window = %v..%v, want the object's lastAvailableDate", b["startDate"], b["endDate"])
		}
		switch b["directorId"] {
		case "FA-1D":
			return `{"resultList":{"result":[{"PercentBusy":11.0,"timestamp":1700000300000}]}}`, true
		case "FA-2D":
			return `{"resultList":{"result":[{"PercentBusy":22.0,"timestamp":1700000300000}]}}`, true
		}
		return "", false
	})
	p := Perf{Cat: feCat(), MaxConcurrent: 2}
	samples, err := p.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2 directors", len(samples))
	}
	byDir := map[string]float64{}
	for _, s := range samples {
		byDir[s.LabelValue("director")] = s.Value
	}
	if byDir["FA-1D"] != 11.0 || byDir["FA-2D"] != 22.0 {
		t.Fatalf("per-director values = %v", byDir)
	}
}

func TestPerfPartialObjectFailureDegradesGracefully(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/FEDirector/keys", `{
	  "feDirectorInfo": [
	    {"directorId": "FA-1D", "lastAvailableDate": 1700000300000},
	    {"directorId": "FA-2D", "lastAvailableDate": 1700000300000}
	  ]
	}`)
	m.SetPostFunc(func(path string, body any) (string, bool) {
		b, ok := body.(map[string]any)
		if !ok {
			return "", false // keys POST uses map[string]string — fall through
		}
		if b["directorId"] == "FA-1D" {
			return `{"resultList":{"result":[{"PercentBusy":11.0,"timestamp":1700000300000}]}}`, true
		}
		return "", false // FA-2D: no response registered -> query error
	})
	p := Perf{Cat: feCat()}
	samples, err := p.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("partial failure must not fail the category: %v", err)
	}
	if len(samples) != 1 || samples[0].LabelValue("director") != "FA-1D" {
		t.Fatalf("samples = %+v, want only FA-1D", samples)
	}
}

func TestPerfAllObjectsFailingFailsCategory(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/FEDirector/keys", `{
	  "feDirectorInfo": [{"directorId": "FA-1D", "lastAvailableDate": 1700000300000}]
	}`)
	// no metrics response registered: every object query fails
	p := Perf{Cat: feCat()}
	if _, err := p.Collect(context.Background(), m, testArrays); err == nil {
		t.Fatal("expected error when every object query fails")
	}
}

func TestPerfLabelKeysConsistentAcrossSeries(t *testing.T) {
	// Family invariant: one metric family carries one label-key set across all
	// series. The engine emits {array} for array-level and {array,<obj>} for
	// object categories — verify per-family key sets are uniform.
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/FEDirector/keys", `{
	  "feDirectorInfo": [
	    {"directorId": "FA-1D", "lastAvailableDate": 1700000300000},
	    {"directorId": "FA-2D", "lastAvailableDate": 1700000300000}
	  ]
	}`)
	m.SetPostFunc(func(path string, _ any) (string, bool) {
		if path != "/univmax/restapi/performance/FEDirector/metrics" {
			return "", false // let the static keys response answer
		}
		return `{"resultList":{"result":[{"PercentBusy":1.0,"timestamp":1700000300000}]}}`, true
	})
	p := Perf{Cat: feCat()}
	samples, err := p.Collect(context.Background(), m,
		[]ArrayWindow{{ID: "A1", Last: 1700000300000}, {ID: "A2", Last: 1700000300000}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	keysOf := func(s Sample) string {
		out := ""
		for _, l := range s.Labels {
			out += l.Key + ","
		}
		return out
	}
	byFamily := map[string]string{}
	for _, s := range samples {
		if prev, ok := byFamily[s.Name]; ok && prev != keysOf(s) {
			t.Fatalf("family %s has divergent label keys: %q vs %q", s.Name, prev, keysOf(s))
		}
		byFamily[s.Name] = keysOf(s)
	}
}
