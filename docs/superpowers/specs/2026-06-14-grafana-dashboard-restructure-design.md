# Grafana dashboard restructure + polish — design

**Date:** 2026-06-14
**Status:** Approved
**Scope:** `grafana/dashboards/pmax-overview.json`, `grafana/dashboards/pmax-lun-deep-dive.json`

## Goal

Rechallenge the two Grafana dashboards to be crisp, professional, focused, and
logically ordered for the **primary viewer: a storage admin / SRE**. The
5-second glance must answer: *is the array healthy, what is it doing, where is
the bottleneck, how full is it.*

## Guardrails

- **No new metrics.** Every panel `expr` references a series the exporter
  already emits (verified against `grep pmax_ internal/`). No exporter,
  catalog, or coupling changes (ADR-0009 dashboard/metric coupling untouched).
- **Family constraints respected:** gauges only, no `rate()` (ADR-0007);
  Unisphere-native units kept; `server` first label, one object label per
  family (ADR-0006).
- Templating, datasource UIDs, and provisioning untouched (only dashboard-level
  cross-links added). `schemaVersion` stays 39; `version` bumped.

## Verified metric inventory (emitted, usable)

Array: `pmax_up`, `pmax_collector_up`, `pmax_array_perf_timestamp_seconds`,
`pmax_array_host_iops`/`_read_iops`/`_write_iops`,
`pmax_array_host_megabytes_per_second`/`_read_`/`_write_`,
`pmax_array_read_response_time_milliseconds`/`_write_`,
`pmax_array_read_percent`/`_write_percent`, `pmax_array_backend_iops`,
`pmax_array_backend_requests_per_second`, `pmax_array_cache_hit_percent`,
`pmax_array_cache_write_pending_percent`, `pmax_array_cache_size_megabytes`,
`pmax_array_device_count`, `pmax_array_disk_count`.
Cache partition: `pmax_cache_partition_hit_percent`/`_host_iops`/
`_host_megabytes_per_second`/`_used_percent`/`_wp_count`/`_wp_utilization_percent`.
FE: `pmax_fe_director_busy_percent`/`_host_iops`/`_host_megabytes_per_second`/
`_queue_depth_utilization_percent`/`_read_response_time_milliseconds`/`_write_`,
`pmax_fe_port_busy_percent`/`_iops`/`_megabytes_per_second`/`_read_`/`_write_`/
`_avg_io_size_kilobytes`/`_response_time_milliseconds`.
BE: `pmax_be_director_busy_percent`/`_iops`/`_read_megabytes_per_second`/`_write_`,
`pmax_be_port_busy_percent`/`_iops`/`_megabytes_per_second`/`_read_`/`_write_`/
`_avg_io_size_kilobytes`.
RDF: `pmax_rdf_director_busy_percent`/`_iops`/`_megabytes_per_second`.
SRP: `pmax_srp_usable_used_terabytes`/`_total_`,
`pmax_srp_subscribed_used_terabytes`/`_total_`,
`pmax_srp_effective_used_capacity_percent`, `pmax_srp_overall_efficiency_ratio`,
`pmax_srp_host_iops`/`_host_megabytes_per_second`/`_response_time_milliseconds`.
SG: `pmax_storage_group_host_iops`/`_host_read_iops`/`_host_write_iops`/
`_host_megabytes_per_second`/`_read_response_time_milliseconds`/`_write_`/
`_allocated_capacity_gigabytes`.
Volume: `pmax_volume_read_iops`/`_write_`/`_read_megabytes_per_second`/`_write_`/
`_read_response_time_milliseconds`/`_write_`/`_allocated_percent`/
`_capacity_gigabytes`/`_info`.

## Overview dashboard

**Header band (always visible):** golden-signals stat strip —
`Unisphere up`, `Arrays`, `Perf data age`, `Failing collectors` (kept) +
`Total host IOPS` (`sum`), `Total host BW` (`sum`), `Worst array RT`
(`max` over read/write response time, red > 2 ms), `Highest SRP used %`.

**Collapsible rows (top-to-bottom = I/O path):**
1. **Array workload** — host IOPS (total/r/w), host bandwidth, read/write mix %,
   backend IOPS.
2. **Latency & cache** — array response time, cache WP% & hit%, cache-partition
   WP utilization.
3. **Front-end path** — FE director busy%, FE director response time, FE
   director queue-depth utilization, FE port busy top-10.
4. **Back-end & replication** — BE director busy%, BE director throughput, RDF
   director busy%, RDF throughput.
5. **Storage groups** — SG IOPS top-10, SG response time top-10, SG bandwidth
   top-10; panels data-link to LUN deep-dive filtered by SG.
6. **Capacity & efficiency** — SRP usable-used bargauge, subscribed /
   oversubscription, efficiency ratio, SRP load.

## LUN deep-dive dashboard

- Keep opt-in banner.
- **Header stat strip:** LUNs reporting (kept) + worst LUN RT + total LUN IOPS +
  highest allocated %.
- **Row "LUN performance":** existing top-20 IOPS / bandwidth / response time /
  allocated panels.
- **Row "Inventory":** identity & capacity table.
- Dashboard link back to Overview, carrying `$server`/`$array`.

## Polish rules (both dashboards)

- `min: 0` on all rate/percent panels; `max: 100` on percent panels.
- Saturation panels (busy%, queue-depth, WP%): threshold step coloring
  green → yellow @70 → red @90.
- Latency panels: threshold coloring green → yellow @1 ms → red @2 ms.
- Legends uniform: `table` placement, `["lastNotNull","max"]` calcs.
- `graphTooltip: 1` (shared crosshair); tooltip `sort: desc`.
- Panel `description` tooltips on non-obvious panels.
- `topk` legends keep the object label.

## Verification

- Both JSON files parse (`jq .`).
- Every `expr` references a metric in the verified inventory above
  (grep cross-check).
- Panel `id`s unique per dashboard; `gridPos` non-overlapping within each row.
