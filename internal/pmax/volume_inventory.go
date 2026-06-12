package pmax

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// volumeListResp is GET /{ver}/sloprovisioning/symmetrix/{id}/volume?storageGroupId=…
// Only the first page (maxPageSize, typically 1000) is read; Count says how many
// exist in total so truncation is loud in the logs (ADR-0009).
type volumeListResp struct {
	Count      int `json:"count"`
	ResultList struct {
		Result []struct {
			VolumeID string `json:"volumeId"`
		} `json:"result"`
	} `json:"resultList"`
}

// volumeDetailResp is the provisional shape of GET …/volume/{volId}. Pointer
// fields keep absent distinct from zero (ADR-0009).
type volumeDetailResp struct {
	VolumeID         string   `json:"volumeId"`
	CapGB            *float64 `json:"cap_gb"`
	AllocatedPercent *float64 `json:"allocated_percent"`
	WWN              string   `json:"wwn"`
	Identifier       string   `json:"volume_identifier"`
	Type             string   `json:"type"`
	StorageGroups    []string `json:"storageGroupId"`
}

// VolumeInventory collects per-volume (LUN) identity and capacity — opt-in via
// collection.volumeInventory: it costs one GET per volume per cycle, bounded by
// the storage-group scope and maxConcurrent.
type VolumeInventory struct {
	Opts          VolumeOptions
	MaxConcurrent int
}

// Name implements ResourceCollector.
func (VolumeInventory) Name() string { return "volume_inventory" }

// Collect implements ResourceCollector.
func (v VolumeInventory) Collect(ctx context.Context, c pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error) {
	var (
		mu              sync.Mutex
		out             []Sample
		queried, failed int
	)
	g, gctx := errgroup.WithContext(ctx)
	limit := v.MaxConcurrent
	if limit <= 0 {
		limit = 8
	}
	g.SetLimit(limit)

	for _, a := range arrays {
		ids, err := v.listVolumes(ctx, c, a)
		if err != nil {
			return nil, err
		}
		base := RestBase + "/" + c.APIVersion() + "/sloprovisioning/symmetrix/" + a.ID + "/volume/"
		for _, id := range ids {
			queried++
			g.Go(func() error {
				var d volumeDetailResp
				err := c.Get(gctx, base+id, &d)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed++
					log.WithFields(log.Fields{
						"server": c.Name(), "array": a.ID, "volume": id, "err": err,
					}).Warn("volume detail query failed")
					return nil // graceful per-volume degradation
				}
				out = append(out, volumeSamples(a.ID, id, d)...)
				return nil
			})
		}
	}
	_ = g.Wait()
	if queried > 0 && failed == queried {
		return nil, fmt.Errorf("volume inventory: all %d detail queries failed", queried)
	}
	return out, nil
}

// listVolumes resolves the deduplicated volume id set across the scoped storage
// groups (a volume can belong to several SGs).
func (v VolumeInventory) listVolumes(ctx context.Context, c pmaxclient.Client, a ArrayWindow) ([]string, error) {
	sgs := v.Opts.StorageGroups
	if len(sgs) == 0 {
		entries, err := postKeys(ctx, c, "StorageGroup", map[string]string{"symmetrixId": a.ID})
		if err != nil {
			return nil, fmt.Errorf("StorageGroup keys for array %s: %w", a.ID, err)
		}
		for _, o := range keyEntries(entries, "storageGroupId", a.Last) {
			sgs = append(sgs, o.id)
		}
	}
	base := RestBase + "/" + c.APIVersion() + "/sloprovisioning/symmetrix/" + a.ID + "/volume"
	seen := map[string]struct{}{}
	for _, sg := range sgs {
		var list volumeListResp
		if err := c.Get(ctx, base+"?storageGroupId="+sg, &list); err != nil {
			return nil, fmt.Errorf("volume list for SG %s: %w", sg, err)
		}
		if list.Count > len(list.ResultList.Result) {
			log.WithFields(log.Fields{
				"server": c.Name(), "array": a.ID, "storage_group": sg,
				"count": list.Count, "page": len(list.ResultList.Result),
			}).Warn("volume list truncated to first page; scope volumeStorageGroups tighter")
		}
		for _, r := range list.ResultList.Result {
			if r.VolumeID != "" {
				seen[r.VolumeID] = struct{}{}
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// volumeSamples maps one volume detail onto samples; absent fields stay absent.
func volumeSamples(arrayID, volID string, d volumeDetailResp) []Sample {
	vl := []Label{{Key: "array", Value: arrayID}, {Key: "volume", Value: volID}}
	var out []Sample
	if d.CapGB != nil {
		out = append(out, Sample{Name: "pmax_volume_capacity_gigabytes", Labels: vl, Value: *d.CapGB})
	}
	if d.AllocatedPercent != nil {
		out = append(out, Sample{Name: "pmax_volume_allocated_percent", Labels: vl, Value: *d.AllocatedPercent})
	}
	out = append(out, Sample{
		Name: "pmax_volume_info",
		Labels: append(vl,
			Label{Key: "wwn", Value: d.WWN},
			Label{Key: "identifier", Value: d.Identifier},
			Label{Key: "type", Value: d.Type},
			Label{Key: "storage_groups", Value: strings.Join(d.StorageGroups, ",")},
		),
		Value: 1,
	})
	return out
}
