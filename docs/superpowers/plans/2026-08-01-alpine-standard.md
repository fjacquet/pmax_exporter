# Alpine Standard — pmax_exporter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert pmax_exporter's published and local container images from `gcr.io/distroless/static:nonroot` to Alpine, matching the family standard, and add the `HEALTHCHECK`/`healthcheck:` this unlocks. Also close a pre-existing, unrelated gap: this is the only repo in the family missing a `docker-compose.ghcr.yml`.

**Architecture:** Both `Dockerfile` and `Dockerfile.goreleaser` swap their final `FROM gcr.io/distroless/static:nonroot` stage for the family's canonical Alpine runtime stage; builder stages (Go version, `ARG VERSION`, ldflags) are untouched. `docker-compose.yml` gains `healthcheck:`. `docker-compose.ghcr.yml` is created new, mirroring `docker-compose.yml`'s topology but pulling the published image instead of building.

**Tech Stack:** Docker, Alpine (`wget`/busybox), Go 1.26.5.

**Spec:** `docs/superpowers/specs/2026-08-01-alpine-standard-design.md` in `obs_exporter` (family-wide design).

## Global Constraints

- `HEALTHCHECK`/`healthcheck:` target `http://127.0.0.1:9443/livez`, never `localhost` — Alpine's busybox `wget` resolves `localhost` via `::1` first, and the exporter only binds IPv4.
- Timing: `--interval=30s --timeout=5s --start-period=10s --retries=3`.
- Builder stages do not change — only the final `FROM` and everything after it.
- Uid `10001`, named user `pmax` (was `nonroot:nonroot`/`65532`) — **this is a breaking change** for the published image; no Helm chart impact (confirmed: `charts/pmax-exporter/values.yaml`'s `runAsUser`/`fsGroup` are commented-out generic defaults, never active).
- `/livez` and `/readyz` are already wired in `main.go` — confirmed, no Go code changes needed.
- No inline `nosemgrep`/`//nolint` suppressions.
- `make ci` must stay green.

## File Structure

| File | Responsibility |
| --- | --- |
| `Dockerfile` | Rewrite runtime stage: distroless → Alpine, add `HEALTHCHECK` |
| `Dockerfile.goreleaser` | Rewrite runtime stage: distroless → Alpine, add `HEALTHCHECK` |
| `docker-compose.yml` | Add `healthcheck:` to the `pmax_exporter` service |
| `docker-compose.ghcr.yml` | New file — pull-based variant |
| `docs/adr/000N-alpine-standard.md` | Records the decision (breaking) |
| `CHANGELOG.md` | `Breaking` entry |

---

### Task 1: Rewrite the local ./Dockerfile to Alpine

**Files:**
- Modify: `Dockerfile`

**Interfaces:** none.

- [ ] **Step 1: Replace the runtime stage**

Current file:

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26.5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/pmax_exporter .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/pmax_exporter /pmax_exporter
USER nonroot:nonroot
EXPOSE 9443
ENTRYPOINT ["/pmax_exporter"]
CMD ["--config", "/etc/pmax_exporter/config.yaml"]
```

Replace with:

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26.5 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/pmax_exporter .

FROM alpine:latest

# Create the runtime user and log dir. These are busybox builtins (no network).
RUN adduser -D -u 10001 pmax && \
    mkdir -p /var/log/pmax_exporter && \
    chown pmax:pmax /var/log/pmax_exporter

# Copy the CA bundle from the builder stage instead of `apk add ca-certificates`.
# The latter fetches from the Alpine CDN over TLS, which fails behind a corporate
# MITM proxy: the bare alpine image has no CA bundle yet to validate the proxy
# cert (chicken-and-egg). The Debian-based golang builder already ships the bundle.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=build /out/pmax_exporter /usr/bin/pmax_exporter
COPY config.yaml /etc/pmax_exporter/config.yaml

EXPOSE 9443

# /livez never depends on target reachability or the collection cycle, so it
# can never flag a healthy process as down over an unreachable Unisphere
# instance (see ADR-000N).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9443/livez || exit 1

USER pmax

ENTRYPOINT ["/usr/bin/pmax_exporter"]
CMD ["--config", "/etc/pmax_exporter/config.yaml"]
```

The local image now bakes `config.yaml` in — it didn't before (distroless Dockerfiles across the family skip this). Intentional, additive.

- [ ] **Step 2: Lint**

Run: `hadolint Dockerfile`
Expected: no findings on the lines just added.

- [ ] **Step 3: Build and verify at runtime**

```bash
docker build -t pmax_exporter:alpine-test .
docker run -d --name pmax-hc-test -p 19443:9443 \
  -e PMAX1_HOSTNAME=unisphere01.example.com -e PMAX1_USERNAME=pmax-monitor -e PMAX1_PASSWORD=changeme \
  pmax_exporter:alpine-test
sleep 15
docker inspect --format='{{.State.Health.Status}}' pmax-hc-test
docker exec pmax-hc-test whoami
```

Expected: `healthy`, `whoami` prints `pmax`.

- [ ] **Step 4: Clean up test artifacts**

```bash
docker rm -f pmax-hc-test
docker rmi pmax_exporter:alpine-test
```

- [ ] **Step 5: Commit**

```bash
git add Dockerfile
git commit -m "feat(docker)!: rewrite local Dockerfile to Alpine (was distroless)

BREAKING CHANGE: container UID changes from 65532 (nonroot) to 10001 (named user pmax)."
```

---

### Task 2: Rewrite Dockerfile.goreleaser to Alpine

**Files:**
- Modify: `Dockerfile.goreleaser`

**Interfaces:** none.

- [ ] **Step 1: Replace the file**

Current file:

```dockerfile
# Release image for GoReleaser (dockers_v2). The binary is cross-compiled by the
# build pipe; buildx lays it out per-platform as ${TARGETPLATFORM}/pmax_exporter in
# the build context. For local/dev builds from source, use the multi-stage ./Dockerfile.
FROM gcr.io/distroless/static:nonroot
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/pmax_exporter /pmax_exporter
COPY config.yaml /etc/pmax_exporter/config.yaml
USER nonroot:nonroot
EXPOSE 9443
ENTRYPOINT ["/pmax_exporter"]
CMD ["--config", "/etc/pmax_exporter/config.yaml"]
```

Replace with:

```dockerfile
# Release image for GoReleaser (dockers_v2). The binary is cross-compiled by the
# build pipe; buildx lays it out per-platform as ${TARGETPLATFORM}/pmax_exporter in
# the build context. For local/dev builds from source, use the multi-stage ./Dockerfile.
FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 10001 pmax && \
    mkdir -p /var/log/pmax_exporter && \
    chown pmax:pmax /var/log/pmax_exporter

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/pmax_exporter /usr/bin/pmax_exporter
COPY config.yaml /etc/pmax_exporter/config.yaml

EXPOSE 9443

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9443/livez || exit 1

USER pmax

ENTRYPOINT ["/usr/bin/pmax_exporter"]
CMD ["--config", "/etc/pmax_exporter/config.yaml"]
```

- [ ] **Step 2: Lint**

Run: `hadolint Dockerfile.goreleaser`
Expected: no new findings.

- [ ] **Step 3: Build and verify at runtime**

```bash
CGO_ENABLED=0 go build -o pmax_exporter .
mkdir -p linux/amd64 && cp pmax_exporter linux/amd64/pmax_exporter
docker build -f Dockerfile.goreleaser --build-arg TARGETPLATFORM=linux/amd64 -t pmax_exporter:goreleaser-test .
docker run -d --name pmax-gr-hc-test -p 19444:9443 \
  -e PMAX1_HOSTNAME=unisphere01.example.com -e PMAX1_USERNAME=pmax-monitor -e PMAX1_PASSWORD=changeme \
  pmax_exporter:goreleaser-test
sleep 15
docker inspect --format='{{.State.Health.Status}}' pmax-gr-hc-test
```

Expected: `healthy`.

- [ ] **Step 4: Clean up test artifacts**

```bash
docker rm -f pmax-gr-hc-test
docker rmi pmax_exporter:goreleaser-test
rm -rf linux pmax_exporter
```

- [ ] **Step 5: Commit**

```bash
git add Dockerfile.goreleaser
git commit -m "feat(docker)!: rewrite the published image to Alpine (was distroless)

BREAKING CHANGE: container UID changes from 65532 (nonroot) to 10001 (named user pmax)."
```

---

### Task 3: Compose — add healthcheck, create the ghcr variant

**Files:**
- Modify: `docker-compose.yml`
- Create: `docker-compose.ghcr.yml`

**Interfaces:** none.

- [ ] **Step 1: Add healthcheck to docker-compose.yml**

In the `pmax_exporter` service, after the `ports:` block (after line 23):

```yaml
    ports:
      - "9443:9443"
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9443/livez"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

- [ ] **Step 2: Create docker-compose.ghcr.yml**

Mirrors `docker-compose.yml`'s topology, pulling the published image instead of building:

```yaml
# Pull-based stack: runs the published GHCR image instead of building locally.
#
#   docker compose -f docker-compose.ghcr.yml up -d
#
# Pin a version with PMAX_TAG (defaults to :latest):
#   PMAX_TAG=0.7.0 docker compose -f docker-compose.ghcr.yml up -d
#
# Refresh images later with:  docker compose -f docker-compose.ghcr.yml pull
services:
  pmax_exporter:
    image: ghcr.io/fjacquet/pmax_exporter:${PMAX_TAG:-latest}
    restart: unless-stopped
    environment:
      PMAX1_HOSTNAME: ${PMAX1_HOSTNAME:-unisphere01.example.com}
      PMAX1_USERNAME: ${PMAX1_USERNAME:-pmax-monitor}
      PMAX1_PASSWORD: ${PMAX1_PASSWORD:-changeme}
      PMAX1_SKIP_CERTIFICATE: ${PMAX1_SKIP_CERTIFICATE:-false}
    volumes:
      - ./config.yaml:/etc/pmax_exporter/config.yaml:ro
    ports:
      - "9443:9443"
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9443/livez"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s

  prometheus:
    image: prom/prometheus:latest
    restart: unless-stopped
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana:latest
    restart: unless-stopped
    environment:
      GF_SECURITY_ADMIN_USER: ${GF_ADMIN_USER:-admin}
      GF_SECURITY_ADMIN_PASSWORD: ${GF_ADMIN_PASSWORD:-admin}
    volumes:
      - ./grafana/provisioning:/etc/grafana/provisioning:ro
      - ./grafana/dashboards:/var/lib/grafana/dashboards:ro
    ports:
      - "3000:3000"
```

- [ ] **Step 3: Validate**

Run: `docker compose -f docker-compose.yml config -q && docker compose -f docker-compose.ghcr.yml config -q`
Expected: both exit 0.

- [ ] **Step 4: Smoke-test docker-compose.yml**

```bash
docker compose up -d --build pmax_exporter
sleep 20
docker inspect --format='{{.State.Health.Status}}' $(docker compose ps -q pmax_exporter)
docker compose down
```

Expected: `healthy`.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml docker-compose.ghcr.yml
git commit -m "feat(docker): add healthcheck; add the missing docker-compose.ghcr.yml"
```

---

### Task 4: ADR + CHANGELOG

**Files:**
- Create: `docs/adr/000N-alpine-standard.md`
- Modify: `CHANGELOG.md`

**Interfaces:** none.

- [ ] **Step 1: Find the next ADR number**

Run: `ls docs/adr/ | sort -V | tail -3`

- [ ] **Step 2: Write the ADR**

```markdown
# Standardize container base image on Alpine

## Status

Accepted (2026-08-01)

## Context

The exporter family had two published-image patterns — Alpine (5 repos) and
`gcr.io/distroless/static:nonroot` (this repo and 2 others: ppdd_exporter,
ppdm_exporter) — as undocumented per-repo author choice, with no written
criterion. Alpine has a shell and `wget`, so it can carry a Docker `HEALTHCHECK`
pointed at `/livez` (already wired in `main.go`); distroless cannot.

## Decision

Both `Dockerfile` and `Dockerfile.goreleaser` move from
`gcr.io/distroless/static:nonroot` to `alpine:latest`. Named user `pmax`, uid
`10001` (was `nonroot:nonroot`/`65532`). `HEALTHCHECK`/`healthcheck:` against
`/livez` via `127.0.0.1` (never `localhost` — Alpine's busybox `wget` resolves
`localhost` via `::1` first, and the exporter only binds IPv4). The
previously-missing `docker-compose.ghcr.yml` is added at the same time.

## Consequences

- **Breaking**: the published image's container UID changes from `65532` to
  `10001`. Checked this repo's Helm chart (`charts/pmax-exporter/values.yaml`)
  for a hardcoded `runAsUser`/`fsGroup` referencing the old UID — none found;
  the chart's security-context fields are commented-out generic defaults,
  never active, so no chart change is required.
- The image gains a shell and `apk` — larger attack surface, larger image —
  accepted family-wide as the trade for `HEALTHCHECK` and shell-based
  debuggability.
- The full family standard and per-repo work breakdown live in
  `obs_exporter`'s `docs/superpowers/specs/2026-08-01-alpine-standard-design.md`.
```

- [ ] **Step 3: Add the CHANGELOG entry**

Under `## [Unreleased]`:

```markdown
### Breaking

- The published Docker image's base changes from
  `gcr.io/distroless/static:nonroot` to `alpine:latest`. The container UID
  changes from `65532` to a named user at `10001`. If you pin `runAsUser`,
  `fsGroup`, or similar in your own deployment manifests against the old UID,
  update it. See ADR-000N.

### Added

- `HEALTHCHECK` on both images, checking `/livez`.
- `docker-compose.ghcr.yml` — was missing; pulls the published image instead
  of building.
```

- [ ] **Step 4: Commit**

```bash
git add docs/adr/000N-alpine-standard.md CHANGELOG.md
git commit -m "docs: record ADR-000N (Alpine standard, breaking UID change)"
```

---

### Task 5: Full gate

- [ ] **Step 1: Run the CI gate**

Run: `make ci`
Expected: clean (no Go code changes, but confirms nothing else regressed).

- [ ] **Step 2: Commit any fixes**

```bash
git commit -am "fix: address CI gate findings for the Alpine standard change"
```

(Skip if the gate was clean.)

## Self-Review

- Spec coverage: pmax_exporter's row in the family table (full conversion, both Dockerfiles; compose healthcheck; create missing ghcr.yml) — Tasks 1–3. Documentation — Task 4.
- No placeholders: ADR number confirmed by a one-command check.
- Breaking change is called out explicitly in both commit messages (`!` + `BREAKING CHANGE:` footer) and the CHANGELOG, per the spec's decision that this is an accepted breaking change.
- Scope: single repo; matches the family plan's per-repo row exactly.
