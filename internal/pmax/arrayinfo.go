package pmax

import (
	"context"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

// symmetrixResp is the provisional shape of GET /{ver}/system/symmetrix/{id}.
// Pointer fields distinguish absent from zero — an unparsed field yields an
// absent sample, never a fake 0 (ADR-0009). `ucode` is the pre-10.x field name;
// 10.x also reports `microcode` — whichever is present wins.
type symmetrixResp struct {
	SymmetrixID string   `json:"symmetrixId"`
	Model       string   `json:"model"`
	Ucode       string   `json:"ucode"`
	Microcode   string   `json:"microcode"`
	DeviceCount *float64 `json:"device_count"`
	DiskCount   *float64 `json:"disk_count"`
	CacheSizeMB *float64 `json:"cache_size_mb"`
}

// ArrayInfo collects identity and inventory gauges per array.
type ArrayInfo struct{}

// Name implements ResourceCollector.
func (ArrayInfo) Name() string { return "array_info" }

// Collect implements ResourceCollector.
func (ArrayInfo) Collect(ctx context.Context, c pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error) {
	var out []Sample
	for _, a := range arrays {
		var sym symmetrixResp
		path := RestBase + "/" + c.APIVersion() + "/system/symmetrix/" + a.ID
		if err := c.Get(ctx, path, &sym); err != nil {
			return out, err
		}
		ucode := sym.Microcode
		if ucode == "" {
			ucode = sym.Ucode
		}
		al := []Label{{Key: "array", Value: a.ID}}
		out = append(out, Sample{
			Name:   "pmax_array_info",
			Labels: append(al, Label{Key: "model", Value: sym.Model}, Label{Key: "ucode", Value: ucode}),
			Value:  1,
		})
		if sym.DeviceCount != nil {
			out = append(out, Sample{Name: "pmax_array_device_count", Labels: al, Value: *sym.DeviceCount})
		}
		if sym.DiskCount != nil {
			out = append(out, Sample{Name: "pmax_array_disk_count", Labels: al, Value: *sym.DiskCount})
		}
		if sym.CacheSizeMB != nil {
			out = append(out, Sample{Name: "pmax_array_cache_size_megabytes", Labels: al, Value: *sym.CacheSizeMB})
		}
	}
	return out, nil
}
