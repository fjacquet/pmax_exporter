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
		return nil, nil
	}
	var out []Sample
	for _, a := range arrays {
		agg := map[fpinKey]*fpinAgg{}
		base := EnhancedBase + "/v1/systems/" + a.ID + "/fpins?select=" + fpinSelect
		path, pages := base, 0
		for {
			var r fpinResp
			if err := c.Get(ctx, path, &r); err != nil {
				if pmaxclient.IsNotFound(err) {
					return out, nil // feature absent: drop this array's partial data, keep earlier arrays
				}
				return out, err
			}
			for _, it := range append(r.Results, r.Fpins...) {
				fpinAdd(agg, it)
			}
			pages++
			if r.Paging.ResumeToken == "" {
				break
			}
			if pages >= fpinMaxPages {
				log.WithField("array", a.ID).Warn("fpin: page cap reached, results truncated")
				break
			}
			path = base + "&resume_token=" + url.QueryEscape(r.Paging.ResumeToken)
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
