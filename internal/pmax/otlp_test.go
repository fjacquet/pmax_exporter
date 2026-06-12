package pmax

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestOTLPObservesSnapshot(t *testing.T) {
	store := NewSnapshotStore()
	store.Store(&Snapshot{BuiltAt: time.Now(), Servers: []*ServerSnapshot{{
		Server: "uni01", OK: true,
		Samples: []Sample{{Name: "pmax_array_host_iops",
			Labels: []Label{{"server", "uni01"}, {"array", "000297900046"}}, Value: 250.5}},
	}}})
	reader := metric.NewManualReader()
	exp := newOTLPExporter(reader, store, "test")
	if err := exp.EnsureInstruments(); err != nil {
		t.Fatalf("EnsureInstruments: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var found bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "pmax_array_host_iops" {
				found = true
				g, ok := m.Data.(metricdata.Gauge[float64])
				if !ok || len(g.DataPoints) != 1 || g.DataPoints[0].Value != 250.5 {
					t.Fatalf("datapoints = %+v", m.Data)
				}
			}
		}
	}
	if !found {
		t.Fatal("pmax_array_host_iops not observed via OTLP ManualReader")
	}
}

func TestOTLPEnsureInstrumentsIdempotent(t *testing.T) {
	store := NewSnapshotStore()
	store.Store(&Snapshot{Servers: []*ServerSnapshot{{
		Samples: []Sample{{Name: "pmax_up", Value: 1}},
	}}})
	reader := metric.NewManualReader()
	exp := newOTLPExporter(reader, store, "test")
	if err := exp.EnsureInstruments(); err != nil {
		t.Fatal(err)
	}
	if err := exp.EnsureInstruments(); err != nil {
		t.Fatalf("second EnsureInstruments must be a no-op: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "pmax_up" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("pmax_up registered %d times, want 1", count)
	}
}
