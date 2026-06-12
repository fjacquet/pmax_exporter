// Command pmax_exporter is a Prometheus + OTLP exporter for Dell PowerMax.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fjacquet/pmax_exporter/internal/config"
	"github.com/fjacquet/pmax_exporter/internal/pmax"
	"github.com/fjacquet/pmax_exporter/internal/pmaxclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var cfgPath string
	var once, debug, trace bool
	root := &cobra.Command{
		Use:     "pmax_exporter",
		Short:   "Prometheus + OTLP exporter for Dell PowerMax",
		Version: version,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(cfgPath, once, debug, trace)
		},
	}
	root.Flags().StringVar(&cfgPath, "config", "config.yaml", "path to config file")
	root.Flags().BoolVar(&once, "once", false, "run a single collection cycle and exit")
	root.Flags().BoolVar(&debug, "debug", false, "verbose logging")
	root.Flags().BoolVar(&trace, "trace", false, "log every Unisphere API response body (live-array payload validation; very verbose)")
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func run(cfgPath string, once, debug, trace bool) error {
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	// Load .env (if present) before interpolation so the `cp .env.example .env`
	// quickstart works for bare-metal runs too; real env vars always win.
	config.LoadDotEnv(cfgPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := pmax.NewSnapshotStore()

	// Optional OTLP metric export (dual export alongside /metrics).
	var otlpExp *pmax.OTLPExporter
	if cfg.OTel.Enabled {
		e, oerr := pmax.NewOTLPExporter(ctx, cfg.OTel.Endpoint, cfg.OTel.Insecure, cfg.OTel.Interval, store, version)
		if oerr != nil {
			log.WithError(oerr).Warn("OTLP export disabled")
		} else {
			otlpExp = e
			defer func() {
				sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = otlpExp.Shutdown(sctx)
			}()
		}
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(pmax.NewPromCollector(store))
	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { healthHandler(w, store) })
	srv := &http.Server{Addr: cfg.Server.Host + ":" + cfg.Server.Port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	// Serve before the first collection cycle (a slow first poll must not block /metrics).
	if !once {
		go func() {
			log.WithField("addr", srv.Addr).Info("serving metrics")
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error(err)
			}
		}()
	}

	// activeLoop owns the targets + cancel func of the currently-running collection
	// loop, so a config reload can stop it and swap in a freshly-built one.
	var activeTargets []pmax.Target
	var activeCancel context.CancelFunc
	stopActive := func() {
		if activeCancel != nil {
			activeCancel()
		}
		for _, t := range activeTargets {
			_ = t.Client.Close()
		}
	}
	defer stopActive()

	startLoop := func(c *config.Config) {
		targets := buildTargets(c, trace)
		col := pmax.NewCollector(targets,
			pmax.Registry(c.Collection.MaxConcurrent, pmax.VolumeOptions{
				Enabled:       c.Collection.VolumeMetrics,
				Inventory:     c.Collection.VolumeInventory,
				StorageGroups: c.Collection.VolumeStorageGroups,
			}),
			store, c.Collection.Interval, c.Collection.Timeout)
		log.Info("running collection cycle")
		col.CollectOnce(ctx)
		if otlpExp != nil {
			if err := otlpExp.EnsureInstruments(); err != nil {
				log.WithError(err).Warn("OTLP instrument registration failed")
			}
		}
		lctx, cancel := context.WithCancel(ctx)
		activeTargets, activeCancel = targets, cancel
		if !once {
			go col.Run(lctx)
		}
	}

	startLoop(cfg)
	if once {
		if debug {
			dumpSamples(store.Load())
		}
		return nil
	}

	if w, werr := config.NewWatcher(cfgPath); werr != nil {
		log.WithError(werr).Warn("config hot-reload disabled")
	} else {
		defer func() { _ = w.Close() }()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case ncfg := <-w.Updates():
					stopActive()
					startLoop(ncfg)
					log.Info("config reloaded; collector rebuilt")
				}
			}
		}()
	}

	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(sctx)
}

// buildTargets constructs a live Unisphere client per configured instance.
func buildTargets(cfg *config.Config, trace bool) []pmax.Target {
	targets := make([]pmax.Target, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		targets = append(targets, pmax.Target{
			Client: pmaxclient.NewServerClient(pmaxclient.Config{
				Name: s.Name, BaseURL: s.BaseURL(), Username: s.Username,
				Password: s.Password, APIVersion: s.APIVersion,
				InsecureSkipVerify: s.InsecureSkipVerify, Trace: trace,
			}),
			Arrays: s.Arrays,
		})
	}
	return targets
}

// dumpSamples prints every collected sample in Prometheus exposition style,
// sorted, so a `--once --debug` run against a live Unisphere can be diffed
// against docs/metrics.md to spot silently-absent metrics.
func dumpSamples(snap *pmax.Snapshot) {
	var lines []string
	for _, sv := range snap.Servers {
		for _, s := range sv.Samples {
			parts := make([]string, 0, len(s.Labels))
			for _, l := range s.Labels {
				parts = append(parts, fmt.Sprintf("%s=%q", l.Key, l.Value))
			}
			lines = append(lines, fmt.Sprintf("%s{%s} %v", s.Name, strings.Join(parts, ","), s.Value))
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Println(l)
	}
}

func healthHandler(w http.ResponseWriter, store *pmax.SnapshotStore) {
	snap := store.Load()
	type serverHealth struct {
		Server     string `json:"server"`
		OK         bool   `json:"ok"`
		LastScrape string `json:"last_scrape"`
		Err        string `json:"err,omitempty"`
	}
	out := struct {
		BuiltAt string         `json:"built_at"`
		Servers []serverHealth `json:"servers"`
	}{BuiltAt: snap.BuiltAt.Format(time.RFC3339)}
	healthy := len(snap.Servers) > 0
	for _, s := range snap.Servers {
		out.Servers = append(out.Servers, serverHealth{s.Server, s.OK, s.LastScrape.Format(time.RFC3339), s.Err})
		if !s.OK {
			healthy = false
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(out)
}
