package pmax

import (
	"context"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

// srpListResp is GET /{ver}/sloprovisioning/symmetrix/{id}/srp.
type srpListResp struct {
	SrpID []string `json:"srpId"`
}

// srpResp is the provisional shape of GET …/srp/{srpId}. Pointer fields keep
// absent distinct from zero (ADR-0009).
type srpResp struct {
	SrpID       string `json:"srpId"`
	SrpCapacity *struct {
		UsableTotalTB    *float64 `json:"usable_total_tb"`
		UsableUsedTB     *float64 `json:"usable_used_tb"`
		SubscribedTotTB  *float64 `json:"subscribed_total_tb"`
		SubscribedUsedTB *float64 `json:"subscribed_used_tb"`
		EffectiveUsedPct *float64 `json:"effective_used_capacity_percent"`
	} `json:"srp_capacity"`
	SrpEfficiency *struct {
		OverallRatioToOne *float64 `json:"overall_efficiency_ratio_to_one"`
	} `json:"srp_efficiency"`
}

// SRPCapacity collects storage-resource-pool capacity and efficiency gauges.
type SRPCapacity struct{}

// Name implements ResourceCollector.
func (SRPCapacity) Name() string { return "srp_capacity" }

// Collect implements ResourceCollector.
func (SRPCapacity) Collect(ctx context.Context, c pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error) {
	var out []Sample
	for _, a := range arrays {
		base := RestBase + "/" + c.APIVersion() + "/sloprovisioning/symmetrix/" + a.ID + "/srp"
		var list srpListResp
		if err := c.Get(ctx, base, &list); err != nil {
			return out, err
		}
		for _, id := range list.SrpID {
			var srp srpResp
			if err := c.Get(ctx, base+"/"+id, &srp); err != nil {
				return out, err
			}
			sl := []Label{{Key: "array", Value: a.ID}, {Key: "srp", Value: id}}
			emit := func(name string, v *float64) {
				if v != nil {
					out = append(out, Sample{Name: name, Labels: sl, Value: *v})
				}
			}
			if srp.SrpCapacity != nil {
				emit("pmax_srp_usable_total_terabytes", srp.SrpCapacity.UsableTotalTB)
				emit("pmax_srp_usable_used_terabytes", srp.SrpCapacity.UsableUsedTB)
				emit("pmax_srp_subscribed_total_terabytes", srp.SrpCapacity.SubscribedTotTB)
				emit("pmax_srp_subscribed_used_terabytes", srp.SrpCapacity.SubscribedUsedTB)
				emit("pmax_srp_effective_used_capacity_percent", srp.SrpCapacity.EffectiveUsedPct)
			}
			if srp.SrpEfficiency != nil {
				emit("pmax_srp_overall_efficiency_ratio", srp.SrpEfficiency.OverallRatioToOne)
			}
		}
	}
	return out, nil
}
