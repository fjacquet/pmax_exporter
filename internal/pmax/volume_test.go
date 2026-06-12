package pmax

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

func TestVolumeBatchesByStorageGroupAndLabels(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostFunc(func(path string, body any) (string, bool) {
		if path != "/univmax/restapi/performance/Volume/metrics" {
			return "", false
		}
		b := body.(map[string]any)
		if b["storageGroups"] != "sg_app1,sg_app2" {
			t.Errorf("storageGroups = %v, want comma-joined chunk", b["storageGroups"])
		}
		if b["dataFormat"] != "Average" {
			t.Errorf("dataFormat = %v", b["dataFormat"])
		}
		return `{"resultList":{"result":[
		  {"volumeId":"00123","storageGroups":"sg_app1","Reads":10.0,"Writes":5.0,"timestamp":1700000300000},
		  {"volumeId":"00124","storageGroups":"sg_app2","Reads":20.0,"timestamp":1700000300000}
		]}}`, true
	})

	v := Volume{Opts: VolumeOptions{Enabled: true, StorageGroups: []string{"sg_app1", "sg_app2"}}}
	samples, err := v.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	byVol := map[string]Sample{}
	for _, s := range samples {
		if s.Name == "pmax_volume_read_iops" {
			byVol[s.LabelValue("volume")] = s
		}
	}
	if byVol["00123"].Value != 10.0 || byVol["00124"].Value != 20.0 {
		t.Fatalf("read iops by volume = %+v", byVol)
	}
	if byVol["00123"].LabelValue("storage_group") != "sg_app1" {
		t.Fatalf("labels = %+v", byVol["00123"].Labels)
	}
	// 00124 has no Writes key — absent, not zero
	for _, s := range samples {
		if s.Name == "pmax_volume_write_iops" && s.LabelValue("volume") == "00124" {
			t.Fatal("absent Writes must not emit a zero sample")
		}
	}
}

func TestVolumeDiscoversStorageGroupsWhenUnset(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostJSON("/univmax/restapi/performance/StorageGroup/keys", `{
	  "storageGroupInfo": [
	    {"storageGroupId": "sg_a", "lastAvailableDate": 1700000300000},
	    {"storageGroupId": "sg_b", "lastAvailableDate": 1700000300000}
	  ]
	}`)
	m.SetPostFunc(func(path string, body any) (string, bool) {
		if path != "/univmax/restapi/performance/Volume/metrics" {
			return "", false
		}
		b := body.(map[string]any)
		if b["storageGroups"] != "sg_a,sg_b" {
			t.Errorf("storageGroups = %v, want discovered SGs", b["storageGroups"])
		}
		return `{"resultList":{"result":[{"volumeId":"001","storageGroups":"sg_a","Reads":1.0,"timestamp":1700000300000}]}}`, true
	})

	v := Volume{Opts: VolumeOptions{Enabled: true}}
	samples, err := v.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
}

func TestVolumeChunksLargeStorageGroupLists(t *testing.T) {
	sgs := make([]string, 25) // 25 SGs -> 3 chunks of <=10
	for i := range sgs {
		sgs[i] = fmt.Sprintf("sg_%02d", i)
	}
	var gotChunks []string
	m := pmaxclient.NewMock("uni01")
	m.SetPostFunc(func(path string, body any) (string, bool) {
		if path != "/univmax/restapi/performance/Volume/metrics" {
			return "", false
		}
		b := body.(map[string]any)
		gotChunks = append(gotChunks, b["storageGroups"].(string))
		return `{"resultList":{"result":[]}}`, true
	})

	v := Volume{Opts: VolumeOptions{Enabled: true, StorageGroups: sgs}, MaxConcurrent: 1}
	if _, err := v.Collect(context.Background(), m, testArrays); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(gotChunks) != 3 {
		t.Fatalf("chunks = %d (%v), want 3", len(gotChunks), gotChunks)
	}
	total := 0
	for _, ch := range gotChunks {
		n := len(strings.Split(ch, ","))
		if n > volumeChunkSize {
			t.Fatalf("chunk too large: %d SGs", n)
		}
		total += n
	}
	if total != 25 {
		t.Fatalf("total SGs across chunks = %d, want 25", total)
	}
}

func TestVolumeKeepsNewestEntryPerVolume(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetPostFunc(func(path string, _ any) (string, bool) {
		if path != "/univmax/restapi/performance/Volume/metrics" {
			return "", false
		}
		resp := map[string]any{"resultList": map[string]any{"result": []map[string]any{
			{"volumeId": "001", "storageGroups": "sg_a", "Reads": 1.0, "timestamp": 1700000000000},
			{"volumeId": "001", "storageGroups": "sg_a", "Reads": 9.0, "timestamp": 1700000300000},
		}}}
		raw, _ := json.Marshal(resp)
		return string(raw), true
	})
	v := Volume{Opts: VolumeOptions{Enabled: true, StorageGroups: []string{"sg_a"}}}
	samples, err := v.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	s := findSample(t, samples, "pmax_volume_read_iops")
	if s.Value != 9.0 {
		t.Fatalf("Reads = %v, want newest 9.0", s.Value)
	}
}
