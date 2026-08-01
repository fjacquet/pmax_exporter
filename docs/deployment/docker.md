# Docker deployment

## Image

`ghcr.io/fjacquet/pmax_exporter` is built `FROM alpine:latest` and contains the
exporter binary, the Alpine/busybox userland (a shell, `wget`, and the rest of
the base image), and its CA bundle. Unlike a distroless image, you *can*
`docker exec -it … sh` into a running container to look around, and the image
ships a Docker `HEALTHCHECK` that runs `wget` against `/livez` from inside the
container every 30 seconds:

```
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9443/livez || exit 1
```

It runs as the named `pmax` user (uid 10001), non-root, multi-arch
(linux/amd64, linux/arm64), published with SBOM + provenance attestations on
every release tag.

## Run

```bash
docker run -d --name pmax_exporter \
  -p 9443:9443 \
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
      - targets: ['pmax_exporter:9443']
```

## Compose

See `docker-compose.yml` at the repo root for the exporter + Prometheus + Grafana
quickstart stack. It builds the exporter image locally (`build: .`).

`docker-compose.ghcr.yml` is the same stack, but pulls the published
`ghcr.io/fjacquet/pmax_exporter` image instead of building it — useful when you
just want to run the demo without a local Go toolchain or Docker build step:

```bash
docker compose -f docker-compose.ghcr.yml up -d
```

Pin a version with `PMAX_TAG` (defaults to `latest`):

```bash
PMAX_TAG=0.7.0 docker compose -f docker-compose.ghcr.yml up -d
```
