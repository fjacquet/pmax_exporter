# FPIN collector — design (short)

## Intent
Unisphere 10.5 adds Fabric Performance Impact Notifications (FPINs): SAN fabric
congestion and link events the array detects per front-end port. Surface them as
metrics and in the overview dashboard so a storage admin sees fabric trouble next to
port workload. Real-time perf is explicitly out (user: an exporter reads values; the
5-minute model stands, ADR-0002).

## Goals / non-goals
- Goal: per-port, per-event-type FPIN count + last-detected time, on by default.
- Goal: zero noise on Unisphere < 10.5 (no error, no `collector_up 0`).
- Non-goal: per-initiator/WWN labels (cardinality grows with hosts).
- Non-goal: rates or event history; the API's retention semantics are unknown.
- Anti-goal: a collector that is permanently red on every pre-10.5 install.

## Approaches (picked: A)
- **A. Always-on, skipped below 10.5** — chosen: cheap (one GET per array),
  safe on old Unisphere, dashboard populated without configuration.
- B. Opt-in flag — rejected: nobody enables it, the panel stays empty.
- C. A + `initiator_wwn` label — rejected: unbounded-ish cardinality.

## Design
- **Collector** `FPIN` (`internal/pmax/fpin.go`), name `fpin`, registered after
  `SRPCapacity` in `Registry`.
- **Gate:** parse `c.APIVersion()` as int; `< 105` → return no samples, nil error.
  A 404 from the endpoint is treated the same (endpoint absent), not a failure.
  Other errors fail the collector as usual.
- **Request:** `GET /univmax/rest/v1/systems/{array}/fpins?select=director_port,fpin_type,detected_time,director`
  — note the Enhanced Endpoints root `/univmax/rest`, not `RestBase`. Follow
  `resume_token` pagination until no token (cap at a fixed page count, log truncation).
- **Decode tolerantly (ADR-0009):** the spec schema says `results` + `paging`, its
  example says `fpins`; accept both. Director from `director.id`, else the part of
  `director_port` before `:`; port = after `:` (matches the FE-port `director`/`port`
  label values).
- **Metrics** (gauges, labels `server, array, director, port, fpin_type`):
  - `pmax_fpin_events` — FPINs currently listed for that port and type.
  - `pmax_fpin_last_detected_timestamp_seconds` — newest `detected_time` / 1000.
  No sample for a port with no FPINs (absent, never zero — ADR-0009).
- **Dashboard** `pmax-overview.json`: new collapsed row "SAN fabric (FPIN)" before
  "Capacity & efficiency": a bar/time series of `sum by (fpin_type)` events and a
  table per `director`/`port`/`fpin_type` with count + last detected (as time ago).
- **Docs:** `docs/metrics.md` rows + label note, CHANGELOG, ADR-0015 note that FPIN is
  adopted (real-time still not), CLAUDE.md architecture line.
- **Tests:** Mock-driven, through Prometheus gather and OTLP reader like existing
  collectors: version 104 → no samples/no error; 404 → no samples/no error; both
  response keys; pagination; `director_port` fallback parsing. Dashboard refs covered
  by the existing spec test. Add a spec check that the FPIN path and the decoded
  fields exist in `14406-10.5.0.json`.

## Left to the builder
Page cap value, panel layout details, Mock support for a query-string path if needed.
