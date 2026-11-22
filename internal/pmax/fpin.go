package pmax

import (
	"context"
	"net/url"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
)

const fpinMaxPages = 20

// fpinResp is GET /univmax/rest/v1/systems/{id}/fpins. The schema names the list
// "results" but the spec example uses "fpins", so both are accepted.
type fpinResp struct {
	Results []fpinItem `json:"results"`
	Fpins   []fpinItem `json:"fpins"`
	Paging  struct {
		ResumeToken string `json:"resume_token"`
	} `json:"paging"`
}

type fpinItem struct {
	DirectorPort string   `json:"director_port"`
	FpinType     string   `json:"fpin_type"`
	DetectedTime *float64 `json:"detected_time"`
	Director     struct {
		ID string `json:"id"`
	} `json:"director"`
}

// fpinSelect lists only fields the 10.5 Enhanced spec offers in `select` (TestFPINInSpec).
const fpinSelect = "director_port,fpin_type,detected_time"

type fpinKey struct{ director, port, typ string }

type fpinAgg struct {
	count float64
	last  *float64
}

// FPIN collects Unisphere 10.5 SAN-fabric congestion notifications per port.
type FPIN struct{}

// Name implements ResourceCollector.
func (FPIN) Name() string { return "fpin" }

// Collect implements ResourceCollector.
func (FPIN) Collect(ctx context.Context, c pmaxclient.Client, arrays []ArrayWindow) ([]Sample, error) {
	if v, err := strconv.Atoi(c.APIVersion()); err == nil && v < 105 {
		ids := make([]string, len(arrays))
		for i, a := range arrays {
			ids[i] = a.ID
		}
		log.WithFields(log.Fields{"server": c.Name(), "array": strings.Join(ids, ","), "apiVersion": v}).
			Info("fpin skipped: REST api version below 105")
		return nil, nil
	}
	var out []Sample
	for _, a := range arrays {
		agg, err := fpinFetch(ctx, c, a)
		if pmaxclient.IsNotFound(err) {
			continue // feature absent for this array: drop its partial data, keep the others
		}
		if err != nil {
			return out, err
		}
		keys := make([]fpinKey, 0, len(agg))
		for k := range agg {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].director != keys[j].director {
				return keys[i].director < keys[j].director
			}
			if keys[i].port != keys[j].port {
				return keys[i].port < keys[j].port
			}
			return keys[i].typ < keys[j].typ
		})
		for _, k := range keys {
			l := []Label{{Key: "array", Value: a.ID}, {Key: "director", Value: k.director},
				{Key: "port", Value: k.port}, {Key: "fpin_type", Value: k.typ}}
			out = append(out, Sample{Name: "pmax_fpin_events", Labels: l, Value: agg[k].count})
			if last := agg[k].last; last != nil {
				out = append(out, Sample{Name: "pmax_fpin_last_detected_timestamp_seconds", Labels: l, Value: *last / 1000})
			}
		}
	}
	return out, nil
}

func fpinFetch(ctx context.Context, c pmaxclient.Client, a ArrayWindow) (map[fpinKey]*fpinAgg, error) {
	agg := map[fpinKey]*fpinAgg{}
	base := EnhancedBase + "/v1/systems/" + a.ID + "/fpins?select=" + fpinSelect
	path, pages := base, 0
	for {
		var r fpinResp
		if err := c.Get(ctx, path, &r); err != nil {
			return nil, err
		}
		items := r.Results
		if len(items) == 0 {
			items = r.Fpins // a server echoing both keys must not double-count
		}
		for _, it := range items {
			fpinAdd(agg, it)
		}
		pages++
		if r.Paging.ResumeToken == "" {
			return agg, nil
		}
		if pages >= fpinMaxPages {
			log.WithField("array", a.ID).Warn("fpin: page cap reached, results truncated")
			return agg, nil
		}
		path = base + "&resume_token=" + url.QueryEscape(r.Paging.ResumeToken)
	}
}

func fpinAdd(agg map[fpinKey]*fpinAgg, it fpinItem) {
	dir, port, _ := strings.Cut(it.DirectorPort, ":")
	if it.Director.ID != "" {
		dir = it.Director.ID
	}
	if (dir == "" && port == "") || it.FpinType == "" {
		return
	}
	k := fpinKey{dir, port, it.FpinType}
	g := agg[k]
	if g == nil {
		g = &fpinAgg{}
		agg[k] = g
	}
	g.count++
	if t := it.DetectedTime; t != nil && (g.last == nil || *t > *g.last) {
		g.last = t
	}
}
