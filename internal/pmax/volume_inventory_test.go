package pmax

import (
	"context"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

func TestVolumeInventoryEmitsCapacityAndInfo(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	base := "/univmax/restapi/100/sloprovisioning/symmetrix/000297900046/volume"
	m.SetJSON(base+"?storageGroupId=sg_app1",
		`{"count": 2, "resultList": {"result": [{"volumeId": "00123"}, {"volumeId": "00124"}]}}`)
	m.SetJSON(base+"/00123", `{
	  "volumeId": "00123", "cap_gb": 100.5, "allocated_percent": 42,
	  "wwn": "60000970000297900046533030313233",
	  "volume_identifier": "oracle_data_01", "type": "TDEV",
	  "storageGroupId": ["sg_app1", "sg_backup"]
	}`)
	m.SetJSON(base+"/00124", `{"volumeId": "00124", "wwn": "60000970000297900046533030313234", "type": "TDEV"}`)

	v := VolumeInventory{Opts: VolumeOptions{Inventory: true, StorageGroups: []string{"sg_app1"}}}
	samples, err := v.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var cap123 *Sample
	for i := range samples {
		s := &samples[i]
		if s.Name == "pmax_volume_capacity_gigabytes" && s.LabelValue("volume") == "00123" {
			cap123 = s
		}
		// 00124 has no cap_gb/allocated_percent — absent, not zero
		if s.LabelValue("volume") == "00124" &&
			(s.Name == "pmax_volume_capacity_gigabytes" || s.Name == "pmax_volume_allocated_percent") {
			t.Fatalf("absent field emitted as sample: %+v", s)
		}
	}
	if cap123 == nil || cap123.Value != 100.5 {
		t.Fatalf("capacity sample = %+v", cap123)
	}

	infos := map[string]Sample{}
	for _, s := range samples {
		if s.Name == "pmax_volume_info" {
			infos[s.LabelValue("volume")] = s
		}
	}
	if len(infos) != 2 {
		t.Fatalf("info samples = %d, want 2", len(infos))
	}
	i123 := infos["00123"]
	if i123.LabelValue("identifier") != "oracle_data_01" ||
		i123.LabelValue("storage_groups") != "sg_app1,sg_backup" ||
		i123.LabelValue("wwn") == "" {
		t.Fatalf("info labels = %+v", i123.Labels)
	}
}

func TestVolumeInventoryDedupesAcrossStorageGroups(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	base := "/univmax/restapi/100/sloprovisioning/symmetrix/000297900046/volume"
	// same volume in both SGs
	m.SetJSON(base+"?storageGroupId=sg_a", `{"count": 1, "resultList": {"result": [{"volumeId": "001"}]}}`)
	m.SetJSON(base+"?storageGroupId=sg_b", `{"count": 1, "resultList": {"result": [{"volumeId": "001"}]}}`)
	m.SetJSON(base+"/001", `{"volumeId": "001", "cap_gb": 10}`)

	v := VolumeInventory{Opts: VolumeOptions{Inventory: true, StorageGroups: []string{"sg_a", "sg_b"}}}
	samples, err := v.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	count := 0
	for _, s := range samples {
		if s.Name == "pmax_volume_capacity_gigabytes" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("capacity samples = %d, want 1 (deduped)", count)
	}
}

func TestVolumeInventoryPartialDetailFailureDegrades(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	base := "/univmax/restapi/100/sloprovisioning/symmetrix/000297900046/volume"
	m.SetJSON(base+"?storageGroupId=sg_a", `{"count": 2, "resultList": {"result": [{"volumeId": "001"}, {"volumeId": "002"}]}}`)
	m.SetJSON(base+"/001", `{"volumeId": "001", "cap_gb": 10}`)
	// 002 detail unregistered -> per-volume failure, collector still succeeds

	v := VolumeInventory{Opts: VolumeOptions{Inventory: true, StorageGroups: []string{"sg_a"}}}
	samples, err := v.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("partial failure must not fail the collector: %v", err)
	}
	if s := findSample(t, samples, "pmax_volume_capacity_gigabytes"); s.LabelValue("volume") != "001" {
		t.Fatalf("unexpected sample: %+v", s)
	}
}
