package pmax

import (
	"context"
	"testing"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

func TestUnisphereInfo(t *testing.T) {
	m := pmaxclient.NewMock("uni01")
	m.SetJSON("/univmax/restapi/version", `{"version": "V10.2.0.1"}`)
	samples, err := Unisphere{}.Collect(context.Background(), m, nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	s := findSample(t, samples, "pmax_unisphere_info")
	if s.Value != 1 || s.LabelValue("version") != "V10.2.0.1" {
		t.Fatalf("unisphere info = %+v", s)
	}
}
