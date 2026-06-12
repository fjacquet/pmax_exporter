# Installation

## Binaries

Download from [GitHub Releases](https://github.com/fjacquet/pmax_exporter/releases):
tar.gz archives and raw binaries for linux/darwin × amd64/arm64, with SHA-256 checksums
and a CycloneDX SBOM.

## Homebrew (macOS)

```bash
brew install --cask fjacquet/tap/pmax_exporter
```

## Container image

```bash
docker pull ghcr.io/fjacquet/pmax_exporter:latest   # pin a version in production
```

## From source

```bash
git clone https://github.com/fjacquet/pmax_exporter
cd pmax_exporter
make cli          # builds bin/pmax_exporter
```

## Unisphere prerequisites

- A read-only Unisphere user (`Monitor` role is sufficient).
- **Performance registration** enabled per array (Unisphere → Performance → Settings →
  System Registrations → Diagnostic). Unregistered arrays expose no performance keys and
  are skipped by discovery.
- Network access to the Unisphere host on port 8443 (TLS).
