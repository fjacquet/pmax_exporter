package pmax

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// volumeMetricDefs is the curated volume catalog (provisional — ADR-0009;
// cross-checked against gopowermax/csm-metrics-powermax volume queries).
var volumeMetricDefs = []MetricDef{
	{Key: "Reads", Name: "pmax_volume_read_iops"},
	{Key: "Writes", Name: "pmax_volume_write_iops"},
	{Key: "MBRead", Name: "pmax_volume_read_megabytes_per_second"},
	{Key: "MBWritten", Name: "pmax_volume_write_megabytes_per_second"},
	{Key: "ReadResponseTime", Name: "pmax_volume_read_response_time_milliseconds"},
	{Key: "WriteResponseTime", Name: "pmax_volume_write_response_time_milliseconds"},
}

// volumeChunkSize is how many storage groups go into one batched Volume
// metrics POST (the endpoint accepts a comma-separated storageGroups list and
// returns per-volume entries — the only batched per-object path Unisphere has).
const volumeChunkSize = 10

// VolumeOptions configures the opt-in volume collector. Volume metrics are
// high-cardinality (one series set per device), so they are disabled unless
// collection.volumeMetrics is set; StorageGroups optionally restricts scope.
type VolumeOptions struct {
	Enabled       bool
	StorageGroups []string
}

// Volume collects per-volume performance, batched by storage group.
type Volume struct {
	Opts          VolumeOptions
	MaxConcurrent int
}

// Name implements ResourceCollector.
func (Volume) Name() string { return "perf_volume" }

// Collect implements ResourceCollector.
func (v Volume) Collect(ctx context.Context, c pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error) {
	var (
		mu             sync.Mutex
		out            []Sample
		chunks, failed int
	)
	g, gctx := errgroup.WithContext(ctx)
	limit := v.MaxConcurrent
	if limit <= 0 {
		limit = 8
	}
	g.SetLimit(limit)

	for _, a := range arrays {
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
		for start := 0; start < len(sgs); start += volumeChunkSize {
			chunk := sgs[start:min(start+volumeChunkSize, len(sgs))]
			chunks++
			g.Go(func() error {
				samples, err := v.queryChunk(gctx, c, a, chunk)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed++
					log.WithFields(log.Fields{
						"server": c.Name(), "array": a.ID,
						"storage_groups": strings.Join(chunk, ","), "err": err,
					}).Warn("volume metrics query failed")
					return nil // graceful per-chunk degradation
				}
				out = append(out, samples...)
				return nil
			})
		}
	}
	_ = g.Wait()
	if chunks > 0 && failed == chunks {
		return nil, fmt.Errorf("volume: all %d chunk queries failed", chunks)
	}
	return out, nil
}

// queryChunk reads the newest datapoint for every volume in a set of storage
// groups via one batched POST.
func (v Volume) queryChunk(ctx context.Context, c pmaxclient.Client, a ArrayWindow, sgs []string) ([]Sample, error) {
	keys := make([]string, len(volumeMetricDefs))
	for i, m := range volumeMetricDefs {
		keys[i] = m.Key
	}
	body := map[string]any{
		"symmetrixId":   a.ID,
		"storageGroups": strings.Join(sgs, ","),
		"startDate":     a.Last,
		"endDate":       a.Last,
		"dataFormat":    "Average",
		"metrics":       keys,
	}
	var resp metricsResp
	if err := c.Post(ctx, RestBase+"/performance/Volume/metrics", body, &resp); err != nil {
		return nil, err
	}
	// One entry per volume per timestamp; keep the newest entry per volume.
	newest := map[string]map[string]any{}
	for _, r := range resp.ResultList.Result {
		id, _ := r["volumeId"].(string)
		if id == "" {
			continue
		}
		ts, _ := toFloat(r["timestamp"])
		if prev, ok := newest[id]; ok {
			if pts, _ := toFloat(prev["timestamp"]); pts >= ts {
				continue
			}
		}
		newest[id] = r
	}
	var out []Sample
	for id, r := range newest {
		sg, _ := r["storageGroups"].(string)
		labels := []Label{
			{Key: "array", Value: a.ID},
			{Key: "storage_group", Value: sg},
			{Key: "volume", Value: id},
		}
		for _, m := range volumeMetricDefs {
			val, ok := toFloat(r[m.Key])
			if !ok {
				continue // absent, never zero
			}
			out = append(out, Sample{Name: m.Name, Labels: labels, Value: val})
		}
	}
	return out, nil
}
