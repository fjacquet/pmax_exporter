# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.6] - 2026-07-10

### Security
- **Bump Go to 1.26.5** to clear `GO-2026-5856` (a `crypto/tls` standard-library
  vulnerability fixed in go1.26.5), which `govulncheck` flags in `make ci`.

### Fixed
- **Multi-arch GHCR image publishing restored.** The `.goreleaser.yaml` was missing a
  `dockers_v2` block, so releases since the image mechanism was dropped published binaries
  but never a container — `ghcr.io/fjacquet/pmax_exporter` was frozen at `0.5.1`. Added the
  `dockers_v2` image (linux/amd64+arm64, SBOM + provenance attestations) and updated
  `Dockerfile.goreleaser` to copy the per-platform `${TARGETPLATFORM}` binary buildx lays out.

## [0.5.5] - 2026-07-03

### Added
- **`pmax_exporter_build_info{version, goversion}` metric.** A constant-`1` gauge whose
  labels carry the running exporter build and Go compiler version, so a single scrape
  confirms which version is actually deployed. Follows the standard Prometheus build-info
  convention (`node_exporter_build_info`, `prometheus_build_info`) and the exporter-family
  standard.

## [0.5.4] - 2026-07-03

### Added
- **systemd deployment.** Shipped a unit file, environment file, and deployment guide for
  running the exporter as a managed system service.

### Changed
- Use the brand icon as the MkDocs favicon and logo.

## [0.5.3] - 2026-06-20

### Changed
- Added standard status badges to the README.
- CI: bumped `actions/checkout` in the GitHub Actions dependency group.

## [0.5.2] - 2026-06-20

### Changed
- CI: migrated to the `fjacquet/ci` reusable, make-based workflows.
- CI: made the `security` job advisory to match the central default.
- Fleet health refresh (tooling/dependency housekeeping).

## [0.5.1] - 2026-06-16

### Added
- **Helm chart** for Kubernetes deployment, with lockstep publishing alongside releases.

## [0.5.0] - 2026-06-14

### Added
- **Node Exporter Full (1860) dashboard** for host-level observability.

### Changed
- **BREAKING:** standardized the metrics endpoint on the canonical port `9443`.

## [0.4.0] - 2026-06-14

### Changed
- Restructured the Grafana dashboards into an SRE I/O-path narrative.

## [0.3.0] - 2026-06-14

### Added
- Windows trace build for live-appliance payload capture.
- Architecture Decision Records (ADRs).

### Changed
- Spec-validated the metric catalog and dashboards against the Unisphere OpenAPI spec.
- Aligned README badges with the canonical exporter-family set.

## [0.2.1] - 2026-06-12

### Changed
- Refreshed dependencies (`x/sync` 0.21.0, `x/net` 0.56.0, `prometheus/common` 0.68.1).

## [0.2.0] - 2026-06-12

### Added
- **LUN (volume) deep-dive:** an inventory collector plus a dedicated Grafana dashboard.

## [0.1.0] - 2026-06-12

### Added
- Initial release: scaffolded `pmax_exporter`, a Dell PowerMax Prometheus + OTLP exporter.
- Front-end/back-end director port and cache-partition category metrics.
- Opt-in volume metrics and an initial Grafana dashboard.
