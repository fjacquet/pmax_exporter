package pmax

import (
	"context"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

// RestBase is the Unisphere REST API root. The /performance namespace below it is
// unversioned; /system and /sloprovisioning take the client's APIVersion prefix.
const RestBase = "/univmax/restapi"

// ArrayWindow is one discovered array and the timestamp (epoch ms) of its newest
// diagnostic performance datapoint, taken from GET /performance/Array/keys.
// Collectors query startDate=endDate=Last so they always read the latest sample.
type ArrayWindow struct {
	ID   string
	Last int64
}

// ResourceCollector collects one metric domain from a single Unisphere instance.
// It returns server-agnostic samples; the loop stamps the `server` label.
// Collectors emit the `array` identity label themselves. Implementations own
// their endpoint paths and JSON structs so provisional-API risk is localized.
type ResourceCollector interface {
	Name() string
	Collect(ctx context.Context, c pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error)
}

// Registry is the ordered set of collectors run for every Unisphere instance.
// maxConcurrent caps in-flight per-object performance POSTs; volume metrics
// are opt-in (high cardinality).
func Registry(maxConcurrent int, vol VolumeOptions) []ResourceCollector {
	out := []ResourceCollector{
		Unisphere{},
		ArrayInfo{},
		SRPCapacity{},
	}
	for _, cat := range PerfCategories() {
		out = append(out, Perf{Cat: cat, MaxConcurrent: maxConcurrent})
	}
	if vol.Enabled {
		out = append(out, Volume{Opts: vol, MaxConcurrent: maxConcurrent})
	}
	if vol.Inventory {
		out = append(out, VolumeInventory{Opts: vol, MaxConcurrent: maxConcurrent})
	}
	return out
}
