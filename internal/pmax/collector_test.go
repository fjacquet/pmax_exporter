package pmax

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

const keysJSON = `{
  "arrayInfo": [
    {"symmetrixId": "000297900046", "firstAvailableDate": 1690000000000, "lastAvailableDate": 1700000300000},
    {"symmetrixId": "000297900047", "firstAvailableDate": 1690000000000, "lastAvailableDate": 1700000300000}
  ]
}`

type fakeCollector struct {
	name    string
	samples []Sample
	err     error
	gotArrs []ArrayWindow
}

func (f *fakeCollector) Name() string { return f.name }
func (f *fakeCollector) Collect(_ context.Context, _ pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error) {
	f.gotArrs = arrays
	return f.samples, f.err
}

func TestCollectOnceStampsServerAndUp(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/performance/Array/keys", keysJSON)
	fc := &fakeCollector{name: "fake", samples: []Sample{
		{Name: "pmax_array_host_iops", Labels: []Label{{Key: "array", Value: "000297900046"}}, Value: 1},
	}}
	store := NewSnapshotStore()
	col := NewCollector([]Target{{Client: m}}, []ResourceCollector{fc}, store, time.Minute, time.Minute)
	snap := col.CollectOnce(context.Background())

	if len(snap.Servers) != 1 || !snap.Servers[0].OK {
		t.Fatalf("server snapshot = %+v", snap.Servers[0])
	}
	if len(fc.gotArrs) != 2 {
		t.Fatalf("collector received %d arrays, want 2 discovered", len(fc.gotArrs))
	}
	up := snap.SamplesByName("pmax_up")
	if len(up) != 1 || up[0].Value != 1 || up[0].LabelValue("server") != "uni01" {
		t.Fatalf("pmax_up = %+v", up)
	}
	ts := snap.SamplesByName("pmax_array_perf_timestamp_seconds")
	if len(ts) != 2 || ts[0].Value != 1700000300 {
		t.Fatalf("perf timestamp samples = %+v", ts)
	}
	cu := snap.SamplesByName("pmax_collector_up")
	if len(cu) != 1 || cu[0].Value != 1 || cu[0].LabelValue("collector") != "fake" {
		t.Fatalf("pmax_collector_up = %+v", cu)
	}
	if s := snap.SamplesByName("pmax_array_host_iops"); len(s) != 1 || s[0].LabelValue("server") != "uni01" {
		t.Fatalf("domain sample not server-stamped: %+v", s)
	}
}

func TestArrayAllowlistFiltersDiscovery(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/performance/Array/keys", keysJSON)
	fc := &fakeCollector{name: "fake"}
	store := NewSnapshotStore()
	col := NewCollector([]Target{{Client: m, Arrays: []string{"000297900047"}}},
		[]ResourceCollector{fc}, store, time.Minute, time.Minute)
	col.CollectOnce(context.Background())

	if len(fc.gotArrs) != 1 || fc.gotArrs[0].ID != "000297900047" {
		t.Fatalf("allowlisted arrays = %+v, want only 000297900047", fc.gotArrs)
	}
}

func TestDiscoveryFailureMarksServerDown(t *testing.T) {
	m := pmaxclient.NewMock("uni01") // no keys response registered
	store := NewSnapshotStore()
	col := NewCollector([]Target{{Client: m}}, []ResourceCollector{&fakeCollector{name: "fake"}},
		store, time.Minute, time.Minute)
	snap := col.CollectOnce(context.Background())

	sv := snap.Servers[0]
	if sv.OK || sv.Err == "" {
		t.Fatalf("server should be down: %+v", sv)
	}
	up := snap.SamplesByName("pmax_up")
	if len(up) != 1 || up[0].Value != 0 {
		t.Fatalf("pmax_up = %+v, want 0", up)
	}
}

func TestFailingCollectorDegradesNotFails(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/performance/Array/keys", keysJSON)
	good := &fakeCollector{name: "good", samples: []Sample{{Name: "pmax_x", Value: 7}}}
	bad := &fakeCollector{name: "bad", err: errors.New("boom")}
	store := NewSnapshotStore()
	col := NewCollector([]Target{{Client: m}}, []ResourceCollector{good, bad}, store, time.Minute, time.Minute)
	snap := col.CollectOnce(context.Background())

	if snap.Servers[0].OK {
		t.Fatal("server must be flagged degraded when a collector fails")
	}
	ups := map[string]float64{}
	for _, s := range snap.SamplesByName("pmax_collector_up") {
		ups[s.LabelValue("collector")] = s.Value
	}
	if ups["good"] != 1 || ups["bad"] != 0 {
		t.Fatalf("collector_up = %v", ups)
	}
	if len(snap.SamplesByName("pmax_x")) != 1 {
		t.Fatal("good collector's samples must survive a sibling failure")
	}
	if len(snap.SamplesByName("pmax_up")) != 1 || snap.SamplesByName("pmax_up")[0].Value != 0 {
		t.Fatal("pmax_up must be 0 when any collector fails")
	}
}

func TestRegistryContainsAllPerfCategories(t *testing.T) {
	reg := Registry(4, VolumeOptions{})
	names := map[string]bool{}
	for _, rc := range reg {
		names[rc.Name()] = true
	}
	for _, want := range []string{
		"unisphere", "array_info", "srp_capacity",
		"perf_array", "perf_fedirector", "perf_bedirector",
		"perf_rdfdirector", "perf_feport", "perf_beport",
		"perf_cachepartition", "perf_storagegroup", "perf_srp",
	} {
		if !names[want] {
			t.Fatalf("registry missing collector %s (have %v)", want, names)
		}
	}
	if names["perf_volume"] {
		t.Fatal("volume collector must be opt-in (disabled by default)")
	}

	reg = Registry(4, VolumeOptions{Enabled: true})
	found := false
	for _, rc := range reg {
		if rc.Name() == "perf_volume" {
			found = true
		}
	}
	if !found {
		t.Fatal("volume collector missing when enabled")
	}
}
