package pmax

import (
	"context"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

func TestSRPCapacityEmitsGauges(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/100/sloprovisioning/symmetrix/000297900046/srp", `{"srpId": ["SRP_1"]}`)
	m.SetJSON("/univmax/restapi/100/sloprovisioning/symmetrix/000297900046/srp/SRP_1", `{
	  "srpId": "SRP_1",
	  "srp_capacity": {
	    "usable_total_tb": 200.5,
	    "usable_used_tb": 120.25,
	    "subscribed_total_tb": 400.0,
	    "subscribed_used_tb": 300.0,
	    "effective_used_capacity_percent": 60
	  },
	  "srp_efficiency": {"overall_efficiency_ratio_to_one": 3.4}
	}`)
	samples, err := SRPCapacity{}.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	s := findSample(t, samples, "pmax_srp_usable_total_terabytes")
	if s.Value != 200.5 || s.LabelValue("srp") != "SRP_1" || s.LabelValue("array") != "000297900046" {
		t.Fatalf("usable_total = %+v", s)
	}
	if s := findSample(t, samples, "pmax_srp_overall_efficiency_ratio"); s.Value != 3.4 {
		t.Fatalf("efficiency = %v", s.Value)
	}
}

func TestSRPAbsentCapacityBlockEmitsNothing(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/100/sloprovisioning/symmetrix/000297900046/srp", `{"srpId": ["SRP_1"]}`)
	m.SetJSON("/univmax/restapi/100/sloprovisioning/symmetrix/000297900046/srp/SRP_1", `{"srpId": "SRP_1"}`)
	samples, err := SRPCapacity{}.Collect(context.Background(), m, testArrays)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("samples = %+v, want none for an SRP without capacity payload", samples)
	}
}
