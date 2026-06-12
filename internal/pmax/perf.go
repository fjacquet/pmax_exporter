package pmax

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Perf is the generic diagnostic-performance collector: one instance per
// category (Array, FEDirector, StorageGroup, …). The Unisphere performance API
// is uniform — POST /performance/{Category}/keys discovers objects, then one
// POST /performance/{Category}/metrics per object reads the newest datapoint —
// so a single engine serves every category (the reason gopowermax was not
// adopted; ADR-0003).
type Perf struct {
	Cat           PerfCategory
	MaxConcurrent int
}

// Name implements ResourceCollector.
func (p Perf) Name() string { return "perf_" + strings.ToLower(p.Cat.Category) }

// metricsResp is POST /performance/{Category}/metrics. Result entries mix the
// requested metric values with a "timestamp" key (epoch ms); values are decoded
// tolerantly — an unparseable value yields an absent sample, never a zero.
type metricsResp struct {
	ResultList struct {
		Result []map[string]any `json:"result"`
	} `json:"resultList"`
}

// Collect implements ResourceCollector.
func (p Perf) Collect(ctx context.Context, c pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error) {
	var (
		mu              sync.Mutex
		out             []Sample
		objects, failed int
	)
	g, gctx := errgroup.WithContext(ctx)
	limit := p.MaxConcurrent
	if limit <= 0 {
		limit = 8
	}
	g.SetLimit(limit)

	for _, a := range arrays {
		objs, err := p.discoverObjects(ctx, c, a)
		if err != nil {
			return nil, fmt.Errorf("%s keys for array %s: %w", p.Cat.Category, a.ID, err)
		}
		for _, o := range objs {
			objects++
			g.Go(func() error {
				samples, err := p.queryObject(gctx, c, a, o)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed++
					log.WithFields(log.Fields{
						"server": c.Name(), "category": p.Cat.Category,
						"array": a.ID, "object": o.id, "err": err,
					}).Warn("performance query failed")
					return nil // graceful per-object degradation
				}
				out = append(out, samples...)
				return nil
			})
		}
	}
	_ = g.Wait()
	if objects > 0 && failed == objects {
		return nil, fmt.Errorf("%s: all %d object queries failed", p.Cat.Category, objects)
	}
	return out, nil
}

// perfObject is one queryable object within an array: the array itself
// (id == "") or a director/storage group/SRP, with its own newest-datapoint
// timestamp from the keys endpoint.
type perfObject struct {
	id   string
	last int64
}

// discoverObjects lists the category's objects for one array. The array-level
// category has exactly one object — the array — and needs no keys call.
func (p Perf) discoverObjects(ctx context.Context, c pmaxclient.Client, a ArrayWindow) ([]perfObject, error) {
	if p.Cat.IDField == "" {
		return []perfObject{{id: "", last: a.Last}}, nil
	}
	// Keys responses wrap the entry list in a single category-specific field
	// (feDirectorInfo, storageGroupInfo, …): decode the wrapper generically and
	// take the only list present, so one parser fits every category.
	var raw map[string][]map[string]any
	if err := c.Post(ctx, RestBase+"/performance/"+p.Cat.Category+"/keys",
		map[string]string{"symmetrixId": a.ID}, &raw); err != nil {
		return nil, err
	}
	var entries []map[string]any
	for _, v := range raw {
		if len(v) > 0 {
			entries = v
			break
		}
	}
	var out []perfObject
	for _, e := range entries {
		id, _ := e[p.Cat.IDField].(string)
		if id == "" {
			continue
		}
		last := a.Last
		if ts, ok := toFloat(e["lastAvailableDate"]); ok && ts > 0 {
			last = int64(ts)
		}
		out = append(out, perfObject{id: id, last: last})
	}
	return out, nil
}

// queryObject reads the newest datapoint for one object and maps the catalog
// metrics onto samples.
func (p Perf) queryObject(ctx context.Context, c pmaxclient.Client, a ArrayWindow, o perfObject) ([]Sample, error) {
	keys := make([]string, len(p.Cat.Metrics))
	for i, m := range p.Cat.Metrics {
		keys[i] = m.Key
	}
	body := map[string]any{
		"symmetrixId": a.ID,
		"startDate":   o.last,
		"endDate":     o.last,
		"dataFormat":  "Average",
		"metrics":     keys,
	}
	if p.Cat.IDField != "" {
		body[p.Cat.IDField] = o.id
	}
	var resp metricsResp
	if err := c.Post(ctx, RestBase+"/performance/"+p.Cat.Category+"/metrics", body, &resp); err != nil {
		return nil, err
	}
	newest := newestResult(resp.ResultList.Result)
	if newest == nil {
		return nil, nil // no datapoint at that timestamp — absent, not zero
	}
	labels := []Label{{Key: "array", Value: a.ID}}
	if p.Cat.IDField != "" {
		labels = append(labels, Label{Key: p.Cat.ObjLabel, Value: o.id})
	}
	var out []Sample
	for _, m := range p.Cat.Metrics {
		v, ok := toFloat(newest[m.Key])
		if !ok {
			continue // absent, never zero
		}
		out = append(out, Sample{Name: m.Name, Labels: labels, Value: v})
	}
	return out, nil
}

// newestResult picks the entry with the highest timestamp — "current" of a time
// series is the newest point, whatever order the API returns.
func newestResult(results []map[string]any) map[string]any {
	var newest map[string]any
	best := -1.0
	for _, r := range results {
		ts, _ := toFloat(r["timestamp"])
		if ts > best {
			best, newest = ts, r
		}
	}
	return newest
}

// toFloat converts a tolerantly-decoded JSON value to float64. Vendor APIs lie
// about shapes (string-typed numbers, "N/A"), so strings are parsed too;
// anything else is absent.
func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}
