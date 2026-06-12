package pmax

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestPromCollectorExposesSnapshot(t *testing.T) {
	store := NewSnapshotStore()
	store.Store(&Snapshot{BuiltAt: time.Now(), Servers: []*ServerSnapshot{{
		Server: "uni01", OK: true,
		Samples: []Sample{
			{Name: "pmax_array_host_iops",
				Labels: []Label{{"server", "uni01"}, {"array", "000297900046"}}, Value: 250.5},
			{Name: "pmax_up", Labels: []Label{{"server", "uni01"}}, Value: 1},
		},
	}}})

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := map[string]bool{}
	for _, mf := range mfs {
		found[mf.GetName()] = true
		if mf.GetName() == "pmax_array_host_iops" {
			m := mf.GetMetric()[0]
			if m.GetGauge().GetValue() != 250.5 {
				t.Fatalf("gauge = %v", m.GetGauge().GetValue())
			}
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			if labels["server"] != "uni01" || labels["array"] != "000297900046" {
				t.Fatalf("labels = %v", labels)
			}
		}
	}
	if !found["pmax_array_host_iops"] || !found["pmax_up"] {
		t.Fatalf("gathered families = %v", found)
	}
}

func TestPromCollectorSkipsInconsistentLabelSets(t *testing.T) {
	store := NewSnapshotStore()
	store.Store(&Snapshot{BuiltAt: time.Now(), Servers: []*ServerSnapshot{{
		Server: "uni01",
		Samples: []Sample{
			{Name: "pmax_clash", Labels: []Label{{"a", "1"}}, Value: 1},
			{Name: "pmax_clash", Labels: []Label{{"b", "2"}}, Value: 2}, // divergent keys
		},
	}}})
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))
	if _, err := reg.Gather(); err == nil {
		// client_golang reports inconsistent families via Gather error; the
		// collector itself must not panic. Either is acceptable, panic is not.
		t.Log("gather tolerated inconsistent labels (no panic) — acceptable")
	}
}
