package pmax

import (
	"context"
	"slices"
	"time"

	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Target couples a Unisphere client with its optional array allowlist
// (symmetrix IDs). An empty allowlist means every performance-registered
// local array discovered on that instance.
type Target struct {
	Client pmaxclient.Client
	Arrays []string
}

// Collector runs the background loop: every interval it polls all Unisphere
// instances in parallel and publishes a fresh Snapshot. One instance's failure
// never blocks others.
type Collector struct {
	targets    []Target
	collectors []ResourceCollector
	store      *SnapshotStore
	interval   time.Duration
	timeout    time.Duration
}

// NewCollector wires the loop.
func NewCollector(targets []Target, collectors []ResourceCollector, store *SnapshotStore, interval, timeout time.Duration) *Collector {
	return &Collector{targets: targets, collectors: collectors, store: store, interval: interval, timeout: timeout}
}

// CollectOnce runs a single cycle, stores, and returns the snapshot.
func (c *Collector) CollectOnce(ctx context.Context) *Snapshot {
	snap := c.collectAll(ctx)
	c.store.Store(snap)
	return snap
}

// Run loops until ctx is cancelled (assumes CollectOnce already primed the store).
func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.store.Store(c.collectAll(ctx))
		}
	}
}

func (c *Collector) collectAll(ctx context.Context) *Snapshot {
	results := make([]*ServerSnapshot, len(c.targets))
	g, gctx := errgroup.WithContext(ctx)
	for i, target := range c.targets {
		g.Go(func() error {
			results[i] = c.collectServer(gctx, target)
			return nil // graceful degradation
		})
	}
	_ = g.Wait()
	return &Snapshot{BuiltAt: time.Now(), Servers: results}
}

// arrayKeysResp is GET /performance/Array/keys. lastAvailableDate is the newest
// diagnostic datapoint (epoch ms); collectors query exactly that timestamp.
type arrayKeysResp struct {
	ArrayInfo []struct {
		SymmetrixID       string `json:"symmetrixId"`
		LastAvailableDate int64  `json:"lastAvailableDate"`
	} `json:"arrayInfo"`
}

// discoverArrays lists the performance-registered local arrays on one Unisphere
// and applies the configured allowlist.
func discoverArrays(ctx context.Context, t Target) ([]ArrayWindow, error) {
	var keys arrayKeysResp
	if err := t.Client.Get(ctx, RestBase+"/performance/Array/keys", &keys); err != nil {
		return nil, err
	}
	var out []ArrayWindow
	for _, a := range keys.ArrayInfo {
		if len(t.Arrays) > 0 && !slices.Contains(t.Arrays, a.SymmetrixID) {
			continue
		}
		out = append(out, ArrayWindow{ID: a.SymmetrixID, Last: a.LastAvailableDate})
	}
	return out, nil
}

func (c *Collector) collectServer(ctx context.Context, target Target) *ServerSnapshot {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	client := target.Client
	ss := &ServerSnapshot{Server: client.Name(), LastScrape: time.Now(), OK: true}

	arrays, err := discoverArrays(ctx, target)
	if err != nil {
		log.WithFields(log.Fields{"server": client.Name(), "err": err}).Warn("array discovery failed")
		ss.OK = false
		ss.Err = err.Error()
		ss.Samples = append(ss.Samples, Sample{Name: "pmax_up", Value: 0}.WithServer(client.Name()))
		return ss
	}
	for _, a := range arrays {
		ss.Samples = append(ss.Samples, Sample{
			Name:   "pmax_array_perf_timestamp_seconds",
			Labels: []Label{{Key: "array", Value: a.ID}},
			Value:  float64(a.Last) / 1000.0,
		}.WithServer(client.Name()))
	}

	serverUp := 1.0
	for _, rc := range c.collectors {
		samples, err := rc.Collect(ctx, client, arrays)
		up := 1.0
		if err != nil {
			up = 0
			serverUp = 0 // any collector failing marks the instance degraded
			log.WithFields(log.Fields{"server": client.Name(), "collector": rc.Name(), "err": err}).
				Warn("collector failed")
		}
		ss.Samples = append(ss.Samples, Sample{
			Name: "pmax_collector_up", Labels: []Label{{Key: "collector", Value: rc.Name()}}, Value: up,
		}.WithServer(client.Name()))
		for _, s := range samples {
			ss.Samples = append(ss.Samples, s.WithServer(client.Name()))
		}
	}
	if serverUp == 0 {
		ss.OK = false
		ss.Err = "one or more collectors failed"
	}
	ss.Samples = append(ss.Samples, Sample{Name: "pmax_up", Value: serverUp}.WithServer(client.Name()))
	return ss
}
