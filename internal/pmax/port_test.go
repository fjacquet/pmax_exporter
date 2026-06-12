package pmax

import (
	"context"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

func fePortCat() PerfCategory {
	return PerfCategory{
		Category: "FEPort", IDField: "portId", ObjLabel: "port",
		Parent:  &PerfParent{Category: "FEDirector", IDField: "directorId", Label: "director"},
		Metrics: []MetricDef{{Key: "PercentBusy", Name: "pmax_fe_port_busy_percent"}},
	}
}

func TestPerfPortNestedDiscovery(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/FEDirector/keys", `{
	  "feDirectorInfo": [
	    {"directorId": "FA-1D", "lastAvailableDate": 1700000300000},
	    {"directorId": "FA-2D", "lastAvailableDate": 1700000300000}
	  ]
	}`)
	m.SetPostFunc(func(path string, body any) (string, bool) {
		b, ok := body.(map[string]string)
		if ok && path == "/univmax/restapi/performance/FEPort/keys" {
			// one port per director, named after it
			switch b["directorId"] {
			case "FA-1D":
				return `{"fePortInfo":[{"portId":"4","lastAvailableDate":1700000300000}]}`, true
			case "FA-2D":
				return `{"fePortInfo":[{"portId":"5","lastAvailableDate":1700000300000}]}`, true
			}
		}
		mb, ok := body.(map[string]any)
		if ok && path == "/univmax/restapi/performance/FEPort/metrics" {
			if mb["directorId"] == "" || mb["portId"] == "" {
				t.Errorf("metrics body missing director/port: %v", mb)
			}
			if mb["directorId"] == "FA-1D" && mb["portId"] == "4" {
				return `{"resultList":{"result":[{"PercentBusy":40.0,"timestamp":1700000300000}]}}`, true
			}
			return `{"resultList":{"result":[{"PercentBusy":50.0,"timestamp":1700000300000}]}}`, true
		}
		return "", false
	})

	p := Perf{Cat: fePortCat(), MaxConcurrent: 2}
	samples, err := p.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2 ports", len(samples))
	}
	byKey := map[string]float64{}
	for _, s := range samples {
		if s.LabelValue("array") != "000297900046" {
			t.Fatalf("missing array label: %+v", s)
		}
		byKey[s.LabelValue("director")+"/"+s.LabelValue("port")] = s.Value
	}
	if byKey["FA-1D/4"] != 40.0 || byKey["FA-2D/5"] != 50.0 {
		t.Fatalf("per-port values = %v", byKey)
	}
}

func TestPerfPortParentKeysFailureFailsCategory(t *testing.T) {
	m := pmaxclient.NewMock("uni01") // no FEDirector keys registered
	p := Perf{Cat: fePortCat()}
	if _, err := p.Collect(context.Background(), m, testArrays); err == nil {
		t.Fatal("expected error when parent keys discovery fails")
	}
}
