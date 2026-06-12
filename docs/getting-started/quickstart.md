# Quick start

## Docker Compose (exporter + Prometheus)

```bash
cp .env.example .env       # set PMAX1_HOSTNAME / PMAX1_USERNAME / PMAX1_PASSWORD
docker compose up -d
open http://localhost:9090 # Prometheus — query: pmax_up
```

## Bare metal

```bash
make cli
cp .env.example .env       # or export the PMAX1_* variables
./bin/pmax_exporter --config config.yaml --debug
curl -s localhost:9104/metrics | grep pmax_up
```

## Validate against your array

The first run against a real Unisphere should validate the provisional metric catalog
(ADR-0009):

```bash
./bin/pmax_exporter --config config.yaml --once --debug --trace 2>trace.log | sort > samples.txt
```

- `samples.txt` — every collected sample; diff against the
  [Metrics reference](../metrics.md).
- `trace.log` — every API response body; if a perf category shows
  `pmax_collector_up 0`, the traced 400 body names the offending metric key.

## Health

```bash
curl -s localhost:9104/health | jq
```

Returns 503 until every configured instance has a healthy collection cycle.
