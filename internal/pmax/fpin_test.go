package pmax

import (
	"context"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

const fpinPath = "/univmax/rest/v1/systems/000297900046/fpins?select=director_port,fpin_type,detected_time,director"

const fpinItems = `[{"director_port":"FA-1D:4","fpin_type":"Congestion","detected_time":1708451234000,"director":{"id":"FA-1D"}},{"director_port":"FA-1D:4","fpin_type":"Congestion","detected_time":1708451299000},{"director_port":"FA-2D:5","fpin_type":"Delivery","detected_time":1708000000000}]`

func fpinMock(v string) *pmaxclient.Mock {
	m := pmaxclient.NewMock("uni01")
	m.SetAPIVersion(v)
	return m
}

func fpinSample(samples []Sample, name, port, typ string) (Sample, bool) {
	for _, s := range samples {
		if s.Name == name && s.LabelValue("port") == port && s.LabelValue("fpin_type") == typ {
			return s, true
		}
	}
	return Sample{}, false
}

func checkFPINAggregates(t *testing.T, m *pmaxclient.Mock) {
	t.Helper()
	samples, err := FPIN{}.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(samples) != 4 {
		t.Fatalf("samples = %+v, want 4", samples)
	}
	s, ok := fpinSample(samples, "pmax_fpin_events", "4", "Congestion")
	if !ok || s.Value != 2 || s.LabelValue("director") != "FA-1D" || s.LabelValue("array") != "000297900046" {
		t.Fatalf("events = %+v", s)
	}
	if s, ok := fpinSample(samples, "pmax_fpin_last_detected_timestamp_seconds", "4", "Congestion"); !ok || s.Value != 1708451299 {
		t.Fatalf("last = %+v", s)
	}
	if s, ok := fpinSample(samples, "pmax_fpin_events", "5", "Delivery"); !ok || s.Value != 1 || s.LabelValue("director") != "FA-2D" {
		t.Fatalf("second = %+v", s)
	}
}

func TestFPINSkipsBefore105(t *testing.T) {
	samples, err := FPIN{}.Collect(context.Background(), fpinMock("104"), testArrays)
	if err != nil || len(samples) != 0 {
		t.Fatalf("samples=%v err=%v, want none", samples, err)
	}
}

func TestFPINNotFoundIsSilent(t *testing.T) {
	m := fpinMock("105")
	m.SetGetError(fpinPath, &pmaxclient.StatusError{Method: "GET", Path: fpinPath, Code: 404})
	samples, err := FPIN{}.Collect(context.Background(), m, testArrays)
	if err != nil || len(samples) != 0 {
		t.Fatalf("samples=%v err=%v, want none", samples, err)
	}
}

func TestFPINOtherErrorFails(t *testing.T) {
	m := fpinMock("105")
	m.SetGetError(fpinPath, &pmaxclient.StatusError{Method: "GET", Path: fpinPath, Code: 500})
	if _, err := (FPIN{}).Collect(context.Background(), m, testArrays); err == nil {
		t.Fatal("want error on 500")
	}
}

func TestFPINAggregatesByPortAndType(t *testing.T) {
	m := fpinMock("105")
	m.SetJSON(fpinPath, `{"results":`+fpinItems+`}`)
	checkFPINAggregates(t, m)
}

func TestFPINAcceptsFpinsKey(t *testing.T) {
	m := fpinMock("105")
	m.SetJSON(fpinPath, `{"fpins":`+fpinItems+`}`)
	checkFPINAggregates(t, m)
}

func TestFPINEmptyListEmitsNothing(t *testing.T) {
	m := fpinMock("105")
	m.SetJSON(fpinPath, `{"results":[]}`)
	samples, err := FPIN{}.Collect(context.Background(), m, testArrays)
	if err != nil || len(samples) != 0 {
		t.Fatalf("samples=%v err=%v, want none", samples, err)
	}
}

func TestFPINFollowsResumeToken(t *testing.T) {
	m := fpinMock("105")
	m.SetJSON(fpinPath, `{"results":[{"director_port":"FA-1D:4","fpin_type":"Congestion","detected_time":1000}],"paging":{"resume_token":"tok1"}}`)
	m.SetJSON(fpinPath+"&resume_token=tok1", `{"results":[{"director_port":"FA-1D:4","fpin_type":"Congestion","detected_time":2000}]}`)
	samples, err := FPIN{}.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if s, ok := fpinSample(samples, "pmax_fpin_events", "4", "Congestion"); !ok || s.Value != 2 {
		t.Fatalf("events = %+v", s)
	}
}

func TestFPINMalformedItemsSkipped(t *testing.T) {
	m := fpinMock("105")
	m.SetJSON(fpinPath, `{"results":[{"director_port":"","fpin_type":"Congestion"},{"director_port":"FA-1D:4","fpin_type":""},{"fpin_type":"Delivery"}]}`)
	samples, err := FPIN{}.Collect(context.Background(), m, testArrays)
	if err != nil || len(samples) != 0 {
		t.Fatalf("samples=%v err=%v, want none", samples, err)
	}
}

func TestRegistryIncludesFPIN(t *testing.T) {
	for _, c := range Registry(1, VolumeOptions{}) {
		if c.Name() == "fpin" {
			return
		}
	}
	t.Fatal("fpin not in Registry")
}

func TestFPINNotFoundKeepsEarlierArrays(t *testing.T) {
	m := fpinMock("105")
	m.SetJSON(fpinPath, `{"results":`+fpinItems+`}`)
	p2 := "/univmax/rest/v1/systems/000297900047/fpins?select=director_port,fpin_type,detected_time,director"
	m.SetGetError(p2, &pmaxclient.StatusError{Method: "GET", Path: p2, Code: 404})
	arrays := []ArrayWindow{testArrays[0], {ID: "000297900047", Last: 1}}
	samples, err := FPIN{}.Collect(context.Background(), m, arrays)
	if err != nil || len(samples) != 4 {
		t.Fatalf("samples=%d err=%v, want 4 from array 1", len(samples), err)
	}
}
