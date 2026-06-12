# Metrics reference

All metrics are **gauges**. Performance values come from Unisphere's diagnostic
(5-minute) data and are already rates/averages — aggregate with `sum`/`avg` in PromQL,
**never `rate()`** (ADR-0007). Metric keys are provisional until validated against a live
Unisphere (ADR-0009); validate with `pmax_exporter --once --debug` and diff against this
page.

## Labels

| Label | On | Meaning |
|---|---|---|
| `server` | every metric | configured Unisphere instance name |
| `array` | every array-scoped metric | symmetrix ID |
| `director` | director and port categories | director ID (e.g. `FA-1D`) |
| `port` | port categories | port ID within the director |
| `cache_partition` | cache-partition category | cache partition ID |
| `storage_group` | storage-group and volume categories | storage group ID |
| `volume` | volume category (opt-in) | device ID |
| `srp` | SRP metrics | storage resource pool ID |

## Exporter health

| Metric | Labels | Description |
|---|---|---|
| `pmax_up` | server | 1 if the instance was reachable and every collector succeeded |
| `pmax_collector_up` | server, collector | per-collector success flag |
| `pmax_array_perf_timestamp_seconds` | server, array | epoch of the newest diagnostic datapoint (alert on staleness) |
| `pmax_unisphere_info` | server, version | always 1; Unisphere version |

## Inventory & capacity

| Metric | Labels | Unit | Source |
|---|---|---|---|
| `pmax_array_info` | server, array, model, ucode | 1 | `GET /{ver}/system/symmetrix/{id}` |
| `pmax_array_device_count` | server, array | count | same |
| `pmax_array_disk_count` | server, array | count | same |
| `pmax_array_cache_size_megabytes` | server, array | MB | same |
| `pmax_srp_usable_total_terabytes` | server, array, srp | TB | `GET …/sloprovisioning/…/srp/{id}` |
| `pmax_srp_usable_used_terabytes` | server, array, srp | TB | same |
| `pmax_srp_subscribed_total_terabytes` | server, array, srp | TB | same |
| `pmax_srp_subscribed_used_terabytes` | server, array, srp | TB | same |
| `pmax_srp_effective_used_capacity_percent` | server, array, srp | % | same |
| `pmax_srp_overall_efficiency_ratio` | server, array, srp | ratio:1 | same |

## Array performance (`POST /performance/Array/metrics`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_array_host_iops` | HostIOs | IO/s |
| `pmax_array_host_read_iops` | HostReads | IO/s |
| `pmax_array_host_write_iops` | HostWrites | IO/s |
| `pmax_array_host_megabytes_per_second` | HostMBs | MB/s |
| `pmax_array_host_read_megabytes_per_second` | HostMBReads | MB/s |
| `pmax_array_host_write_megabytes_per_second` | HostMBWritten | MB/s |
| `pmax_array_read_response_time_milliseconds` | ReadResponseTime | ms |
| `pmax_array_write_response_time_milliseconds` | WriteResponseTime | ms |
| `pmax_array_backend_iops` | BEIOs | IO/s |
| `pmax_array_backend_requests_per_second` | BEReqs | req/s |
| `pmax_array_cache_write_pending_percent` | PercentCacheWP | % |
| `pmax_array_cache_hit_percent` | PercentHit | % |
| `pmax_array_read_percent` | PercentReads | % |
| `pmax_array_write_percent` | PercentWrites | % |

## FE director performance (`/performance/FEDirector`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_fe_director_busy_percent` | PercentBusy | % |
| `pmax_fe_director_host_iops` | HostIOs | IO/s |
| `pmax_fe_director_host_megabytes_per_second` | HostMBs | MB/s |
| `pmax_fe_director_read_response_time_milliseconds` | ReadResponseTime | ms |
| `pmax_fe_director_write_response_time_milliseconds` | WriteResponseTime | ms |
| `pmax_fe_director_queue_depth_utilization_percent` | QueueDepthUtilization | % |

## BE director performance (`/performance/BEDirector`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_be_director_busy_percent` | PercentBusy | % |
| `pmax_be_director_iops` | IOs | IO/s |
| `pmax_be_director_read_megabytes_per_second` | MBReads | MB/s |
| `pmax_be_director_write_megabytes_per_second` | MBWritten | MB/s |

## RDF director performance (`/performance/RDFDirector`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_rdf_director_busy_percent` | PercentBusy | % |
| `pmax_rdf_director_iops` | IOs | IO/s |
| `pmax_rdf_director_megabytes_per_second` | MBSentAndReceived | MB/s |

## FE port performance (`/performance/FEPort`)

Discovered per FE director (`keys` POST carries `directorId`); labels `director` + `port`.

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_fe_port_busy_percent` | PercentBusy | % |
| `pmax_fe_port_iops` | IOs | IO/s |
| `pmax_fe_port_megabytes_per_second` | MBs | MB/s |
| `pmax_fe_port_read_megabytes_per_second` | MBRead | MB/s |
| `pmax_fe_port_write_megabytes_per_second` | MBWritten | MB/s |
| `pmax_fe_port_response_time_milliseconds` | ResponseTime | ms |
| `pmax_fe_port_avg_io_size_kilobytes` | AvgIOSize | KB |

## BE port performance (`/performance/BEPort`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_be_port_busy_percent` | PercentBusy | % |
| `pmax_be_port_iops` | IOs | IO/s |
| `pmax_be_port_megabytes_per_second` | MBs | MB/s |
| `pmax_be_port_read_megabytes_per_second` | MBRead | MB/s |
| `pmax_be_port_write_megabytes_per_second` | MBWritten | MB/s |
| `pmax_be_port_avg_io_size_kilobytes` | AvgIOSize | KB |

## Cache partition performance (`/performance/CachePartition`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_cache_partition_wp_count` | WPCount | slots |
| `pmax_cache_partition_used_percent` | PercentCacheUsed | % |
| `pmax_cache_partition_wp_utilization_percent` | PercentWPUtilization | % |
| `pmax_cache_partition_host_iops` | HostIOs | IO/s |
| `pmax_cache_partition_host_megabytes_per_second` | HostMBs | MB/s |
| `pmax_cache_partition_hit_percent` | PercentHit | % |

## Storage group performance (`/performance/StorageGroup`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_storage_group_host_iops` | HostIOs | IO/s |
| `pmax_storage_group_host_read_iops` | HostReads | IO/s |
| `pmax_storage_group_host_write_iops` | HostWrites | IO/s |
| `pmax_storage_group_host_megabytes_per_second` | HostMBs | MB/s |
| `pmax_storage_group_read_response_time_milliseconds` | ReadResponseTime | ms |
| `pmax_storage_group_write_response_time_milliseconds` | WriteResponseTime | ms |
| `pmax_storage_group_allocated_capacity_gigabytes` | AllocatedCapacity | GB |

## SRP performance (`/performance/SRP`)

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_srp_host_iops` | HostIOs | IO/s |
| `pmax_srp_host_megabytes_per_second` | HostMBs | MB/s |
| `pmax_srp_response_time_milliseconds` | ResponseTime | ms |

## Volume performance (`/performance/Volume`) — opt-in

**Disabled by default** (one series set per device — high cardinality). Enable with
`collection.volumeMetrics: true`; scope with `collection.volumeStorageGroups`. Queries
are batched: one POST per ≤10 storage groups returns per-volume entries. Labels:
`array`, `storage_group`, `volume`.

| Metric | Unisphere key | Unit |
|---|---|---|
| `pmax_volume_read_iops` | Reads | IO/s |
| `pmax_volume_write_iops` | Writes | IO/s |
| `pmax_volume_read_megabytes_per_second` | MBRead | MB/s |
| `pmax_volume_write_megabytes_per_second` | MBWritten | MB/s |
| `pmax_volume_read_response_time_milliseconds` | ReadResponseTime | ms |
| `pmax_volume_write_response_time_milliseconds` | WriteResponseTime | ms |

## Deferred (backlog)

RDF ports, real-time (1-minute) performance API.
