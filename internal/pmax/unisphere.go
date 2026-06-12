package pmax

import (
	"context"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

// versionResp is GET /univmax/restapi/version. Validated against Unisphere 10.x.
type versionResp struct {
	Version string `json:"version"` // e.g. "V10.2.0.1"
}

// Unisphere emits one info gauge for the management instance itself.
type Unisphere struct{}

// Name implements ResourceCollector.
func (Unisphere) Name() string { return "unisphere" }

// Collect implements ResourceCollector.
func (Unisphere) Collect(ctx context.Context, c pmaxclient.Client, _ []ArrayWindow) ([]Sample, error) {
	var v versionResp
	if err := c.Get(ctx, RestBase+"/version", &v); err != nil {
		return nil, err
	}
	return []Sample{{
		Name:   "pmax_unisphere_info",
		Labels: []Label{{Key: "version", Value: v.Version}},
		Value:  1,
	}}, nil
}
