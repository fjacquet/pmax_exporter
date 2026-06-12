package pmax

import "testing"

func TestWithServerPrependsLabel(t *testing.T) {
	s := Sample{Name: "pmax_x", Labels: []Label{{Key: "array", Value: "A1"}}, Value: 1}
	out := s.WithServer("uni01")
	if len(out.Labels) != 2 || out.Labels[0].Key != "server" || out.Labels[0].Value != "uni01" {
		t.Fatalf("labels = %+v", out.Labels)
	}
	if s.LabelValue("server") != "" {
		t.Fatal("WithServer must not mutate the original sample")
	}
	if out.LabelValue("array") != "A1" {
		t.Fatal("existing labels must be preserved")
	}
}

func TestSnapshotMetricNamesSortedUnique(t *testing.T) {
	snap := &Snapshot{Servers: []*ServerSnapshot{
		{Samples: []Sample{{Name: "pmax_b"}, {Name: "pmax_a"}}},
		{Samples: []Sample{{Name: "pmax_a"}}},
	}}
	names := snap.MetricNames()
	if len(names) != 2 || names[0] != "pmax_a" || names[1] != "pmax_b" {
		t.Fatalf("names = %v", names)
	}
}
