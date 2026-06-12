# Docker deployment

## Image

`ghcr.io/fjacquet/pmax_exporter` — distroless static, non-root, multi-arch
(linux/amd64, linux/arm64), published with SBOM + provenance attestations on every
release tag.

## Run

```bash
docker run -d --name pmax_exporter \
  -p 9104:9104 \
  -v $(pwd)/config.yaml:/etc/pmax_exporter/config.yaml:ro \
  -e PMAX1_HOSTNAME=unisphere01.example.com \
  -e PMAX1_USERNAME=pmax-monitor \
  -e PMAX1_PASSWORD=… \
  ghcr.io/fjacquet/pmax_exporter:latest
```

Secrets: prefer `passwordFile` + a mounted secret over environment variables where your
platform supports it (Docker/Podman secrets, Kubernetes projected volumes).

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: pmax_exporter
    scrape_interval: 1m        # snapshot refreshes every 5m; 1m scrapes are cheap reads
    static_configs:
      - targets: ['pmax_exporter:9104']
```

## Compose

See `docker-compose.yml` at the repo root for the exporter + Prometheus quickstart stack.
