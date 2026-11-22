# Unisphere 10.5 canonical spec; auto-detected REST version

## Status

Accepted (2026-11-22). Updates the canonical-spec choice of
[ADR-0010](0010-spec-validation-ci-gate.md); the gate's checks are unchanged.

## Context

Dell published the Unisphere for PowerMax 10.5 OpenAPI specs (`14405-10.5.0.json`,
`14406-10.5.0.json` Enhanced Endpoints). Diffing them against the 10.4 spec ADR-0010
vendored:

- **Performance:** same 51 `/performance/{Cat}/metrics` categories, every metric enum
  identical. Every catalog and volume key still validates; no dashboard query changes.
- **Volume and SRP schemas:** identical property sets.
- **`systemSymmetrix`:** 10.4 documented mixed-case names (`cache_Size_Mb`); 10.5
  documents snake_case (`cache_size_mb`), the exact tags `arrayinfo.go` uses. Decoding
  never depended on this (`encoding/json` matches names case-insensitively), but the
  gate's exact-case membership check could not cover these fields against 10.4. It can
  against 10.5.
- **Removed:** the `/{ver}/file` (eNAS) namespace, 135 paths. The exporter never used it.
- **New, not adopted:** Enhanced Endpoints `/v1/systems/{id}/realtime-categories` and
  `/fpins`. Real-time (seconds) data conflicts with the 5-minute diagnostic-granularity
  default (ADR-0002), so it needs its own decision.
- **Versioned prefix:** the 10.5 spec documents only `/105/...`. Dell's 10.0–10.2 notes
  describe N-2 REST version support, so a 10.5 server is expected to drop `/100/`, which
  was the exporter's fixed default. That is extrapolated, not confirmed for 10.5.

## Decision

1. `docs/swagger/14405-10.5.0.json` is the canonical spec for `catalog_spec_test.go`;
   9.2 remains the skew cross-check. The constant is version-neutral (`specPath`) so the
   next release is a one-line change. The 10.4 files stay vendored for reference.
2. Add `TestArrayInfoFieldsInSpec` for the `/system/symmetrix` fields `arrayinfo.go`
   decodes (`ucode`, the pre-10.x alias, is excluded).
3. `apiVersion` is optional. When unset, `ServerClient.APIVersion()` reads
   `api_version` from `GET /univmax/restapi/version` (unauthenticated in the spec, sent
   with Basic auth like every call) and caches it. On failure it logs a warning, uses
   `100` for that call and retries next call, so a transiently unreachable server
   recovers without a restart. An explicit `apiVersion` always wins and skips detection.
   Only `/system` and `/sloprovisioning` calls use the prefix; `/performance` is
   unversioned and unaffected.

## Consequences

- Upgrading Unisphere no longer silently breaks array info, SRP capacity and volume
  inventory.
- One extra GET per server per client lifetime (a config reload builds a new client and
  re-detects).
- While detection fails, the fallback blocks for at most 30 s per call, serialized per
  server.
- Live-array validation (ADR-0009) is still pending; spec agreement is not proof.
