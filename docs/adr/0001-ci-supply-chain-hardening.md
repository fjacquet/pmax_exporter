# CI & supply-chain hardening

## Status
Accepted.

## Context
The exporter family standard requires reproducible, tamper-resistant builds: CI must be
re-runnable locally, dependencies and actions must be pinned, and every artifact must ship
with a software bill of materials.

## Decision
- **Everything CI runs is a Makefile target** (`make ci` = gofmt check, `go vet`,
  `golangci-lint`, `go test -race`, `govulncheck`, build).
- GitHub Actions are **SHA-pinned** with an explicit `# vX.Y.Z` comment; Dependabot bumps
  both (actions, gomod, docker ecosystems).
- `persist-credentials: false` on every checkout; `cache: false` on setup-go in
  `release.yml` (release artifacts must not be built from a restorable cache); GitHub
  Pages permissions scoped to the deploy job only.
- Releases via **GoReleaser**: CGO off, linux/darwin × amd64/arm64, `-trimpath`,
  commit-timestamp stamping, SHA-256 checksums, CycloneDX SBOM
  (`cyclonedx-gomod`), GitHub-native changelog, self-skipping Homebrew cask.
- Container image: distroless static, non-root `USER`, multi-arch, BuildKit SBOM +
  provenance attestations.
- Semgrep runs in CI and on every local file write; inline suppressions
  (`// nosemgrep`, `//nolint`) are not allowed — restructure instead.

## Consequences
CI failures reproduce locally with one command. A compromised upstream action or
dependency bump is visible in review as a SHA change. Every release asset is attributable
and auditable via SBOM + provenance.
