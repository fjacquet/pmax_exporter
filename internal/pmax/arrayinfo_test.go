package pmax

import (
	"context"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

func TestArrayInfoEmitsIdentityAndInventory(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/100/system/symmetrix/000297900046", `{
	  "symmetrixId": "000297900046",
	  "model": "PowerMax_2500",
	  "microcode": "6079.275.0",
	  "device_count": 1234,
	  "disk_count": 96,
	  "cache_size_mb": 1048576
	}`)
	samples, err := ArrayInfo{}.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	info := findSample(t, samples, "pmax_array_info")
	if info.LabelValue("model") != "PowerMax_2500" || info.LabelValue("ucode") != "6079.275.0" {
		t.Fatalf("info labels = %+v", info.Labels)
	}
	if s := findSample(t, samples, "pmax_array_device_count"); s.Value != 1234 {
		t.Fatalf("device_count = %v", s.Value)
	}
	if s := findSample(t, samples, "pmax_array_cache_size_megabytes"); s.Value != 1048576 {
		t.Fatalf("cache_size_mb = %v", s.Value)
	}
}

func TestArrayInfoFallsBackToUcodeField(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/100/system/symmetrix/000297900046", `{
	  "symmetrixId": "000297900046", "model": "PowerMax_2000", "ucode": "5978.711.711"
	}`)
	samples, err := ArrayInfo{}.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	info := findSample(t, samples, "pmax_array_info")
	if info.LabelValue("ucode") != "5978.711.711" {
		t.Fatalf("ucode fallback = %q", info.LabelValue("ucode"))
	}
	// absent inventory fields must yield absent samples, never zeros
	for _, s := range samples {
		if s.Name == "pmax_array_device_count" {
			t.Fatal("absent device_count must not be emitted as zero")
		}
	}
}
