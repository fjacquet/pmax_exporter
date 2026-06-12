# Configuration

`config.yaml` is the source of truth. `${ENV_VAR}` references in `host`, `username`, and
`password` are interpolated at load (and on every reload); an unset variable fails the
load fast. A `.env` file next to the config (or in the working directory) is loaded
natively — already-set environment variables always win.

```yaml
server:
  host: "0.0.0.0"
  port: "9104"
  uri: "/metrics"

collection:
  interval: "5m"      # diagnostic perf granularity is 5 min — faster polling re-reads the same point
  timeout: "120s"     # per-instance cycle budget
  maxConcurrent: 8    # cap on in-flight per-object performance POSTs per instance

otel:
  enabled: false      # optional OTLP gRPC metric push (dual export)
  endpoint: "localhost:4317"
  insecure: true
  interval: "30s"

servers:
  - name: unisphere-prod-01
    host: "${PMAX1_HOSTNAME}"
    port: 8443
    username: "${PMAX1_USERNAME}"
    password: "${PMAX1_PASSWORD}"   # or passwordFile: /run/secrets/pmax1
    insecureSkipVerify: true        # explicit opt-in for self-signed Unisphere certs
    apiVersion: "100"               # REST version prefix for /system & /sloprovisioning
    arrays: []                      # optional symmetrixId allowlist; empty = all local perf-registered arrays
```

## Multiple Unisphere instances

Add one `servers:` entry per instance. The `name` becomes the `server` label.

## Hot reload

Edit the file (or `kill -HUP <pid>`): clients and the collection loop are rebuilt and
swapped without dropping the HTTP endpoint. Invalid configs are rejected and logged; the
running config stays.

## CLI flags

| Flag | Purpose |
|---|---|
| `--config` | config file path (default `config.yaml`) |
| `--debug` | verbose logging |
| `--once` | one collection cycle, then exit (with `--debug`: dump every sample, sorted) |
| `--trace` | log every API response body — live payload validation; never logs headers/credentials |
