# Windows binary for trace capture

## Status
Accepted.

## Context
`pmax_exporter` is a long-running server, normally deployed as a Linux container or
systemd unit — so releases were linux/darwin only (`.goreleaser.yaml`). But the one
outstanding risk in [ADR-0009](0009-provisional-api-mappings.md) / 
[ADR-0010](0010-spec-validation-ci-gate.md) is the live-array run, and the people with
network access to a production Unisphere are often storage admins on Windows
workstations. Asking them to stand up a Linux box just to run
`pmax_exporter --config real.yaml --once --trace` is friction that delays validation.

## Decision
- Add `windows` to the GoReleaser `goos` matrix, shipping a **windows/amd64** archive as a
  `.zip` (platform convention) bundling `pmax_exporter.exe` + LICENSE/README/config.
- Ignore **windows/arm64** — not a real target for this workflow.
- No code changes were needed: the build is CGO-disabled and the SIGHUP hot-reload
  (ADR-0005) is already platform-guarded, so windows/amd64 cross-compiles clean.

## Consequences
- A Windows admin can download one `.exe` and run `--once --trace` to capture the live
  Unisphere bodies ADR-0009/0010 need, with no Linux dependency.
- Windows is a **diagnostic/trace target, not a supported deployment target** — production
  still runs the Linux container or a darwin/linux binary. We do not test Windows in CI
  beyond the cross-compile that GoReleaser performs at release time.
- The release matrix grows by one artifact; the Homebrew cask and GHCR image are
  unaffected (macOS / Linux only).
