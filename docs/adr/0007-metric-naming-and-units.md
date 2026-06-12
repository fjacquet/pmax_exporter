# Metric naming & units

## Status
Accepted.

## Context
Unisphere performance values are already **rates and averages** over the 5-minute
diagnostic window (IO/s, MB/s, milliseconds, percent). Exposing them as counters or with
ambiguous units invites `rate()` misuse and unit confusion in dashboards.

## Decision
- **Everything is a gauge.** Per-second values (`_iops`, `_megabytes_per_second`,
  `_requests_per_second`) are aggregated with `sum`/`avg` in PromQL — **never `rate()`**.
- Names are **unit-explicit** and keep Unisphere's native units: megabytes per second
  (not bytes — the API reports MB/s), `_milliseconds` for response times, `_percent` for
  percentages, `_terabytes` for SRP capacity (API-native TB), `_gigabytes` for storage
  group allocated capacity (API-native GB).
- Prefix `pmax_`, then object scope: `pmax_array_*`, `pmax_fe_director_*`,
  `pmax_be_director_*`, `pmax_rdf_director_*`, `pmax_storage_group_*`, `pmax_srp_*`,
  plus exporter-health families `pmax_up`, `pmax_collector_up`,
  `pmax_array_perf_timestamp_seconds`, and info gauges `pmax_unisphere_info`,
  `pmax_array_info`.

## Consequences
No unit conversion bugs between the API and the exporter — what Unisphere reports is what
the metric name says. Converting to SI bytes (if ever wanted) is a deliberate dashboard
decision (`* 1024 * 1024`), not hidden exporter magic.
