package pmax

// MetricDef maps one Unisphere performance metric key to its exported,
// unit-explicit name. Per-second values (IOPS, MB/s) and response times are
// already rates/averages — gauges, aggregated with sum/avg in PromQL, never
// rate() (family naming/units rule, ADR-0007).
type MetricDef struct {
	Key  string // Unisphere metric name, exact case (a wrong key 400s the whole query)
	Name string // exported metric name
}

// PerfCategory describes one /performance/{Category} namespace: how its objects
// are keyed and which curated metrics to query. The catalog is provisional
// until validated against a live Unisphere with `--once --debug` (ADR-0009).
type PerfCategory struct {
	Category string // URL path segment, e.g. "FEDirector"
	IDField  string // object id field in keys entries + metrics body; "" = array-level
	ObjLabel string // Prometheus label for the object id; "" = array-level
	Metrics  []MetricDef
}

// PerfCategories is the v1 catalog: Array, FE/BE/RDF directors, StorageGroup,
// SRP. FEPort/BEPort are deferred (their keys endpoints are per-director).
func PerfCategories() []PerfCategory {
	return []PerfCategory{
		{
			Category: "Array",
			Metrics: []MetricDef{
				{Key: "HostIOs", Name: "pmax_array_host_iops"},
				{Key: "HostReads", Name: "pmax_array_host_read_iops"},
				{Key: "HostWrites", Name: "pmax_array_host_write_iops"},
				{Key: "HostMBs", Name: "pmax_array_host_megabytes_per_second"},
				{Key: "HostMBReads", Name: "pmax_array_host_read_megabytes_per_second"},
				{Key: "HostMBWritten", Name: "pmax_array_host_write_megabytes_per_second"},
				{Key: "ReadResponseTime", Name: "pmax_array_read_response_time_milliseconds"},
				{Key: "WriteResponseTime", Name: "pmax_array_write_response_time_milliseconds"},
				{Key: "BEIOs", Name: "pmax_array_backend_iops"},
				{Key: "BEReqs", Name: "pmax_array_backend_requests_per_second"},
				{Key: "PercentCacheWP", Name: "pmax_array_cache_write_pending_percent"},
				{Key: "PercentHit", Name: "pmax_array_cache_hit_percent"},
				{Key: "PercentReads", Name: "pmax_array_read_percent"},
				{Key: "PercentWrites", Name: "pmax_array_write_percent"},
			},
		},
		{
			Category: "FEDirector",
			IDField:  "directorId",
			ObjLabel: "director",
			Metrics: []MetricDef{
				{Key: "PercentBusy", Name: "pmax_fe_director_busy_percent"},
				{Key: "HostIOs", Name: "pmax_fe_director_host_iops"},
				{Key: "HostMBs", Name: "pmax_fe_director_host_megabytes_per_second"},
				{Key: "ReadResponseTime", Name: "pmax_fe_director_read_response_time_milliseconds"},
				{Key: "WriteResponseTime", Name: "pmax_fe_director_write_response_time_milliseconds"},
				{Key: "QueueDepthUtilization", Name: "pmax_fe_director_queue_depth_utilization_percent"},
			},
		},
		{
			Category: "BEDirector",
			IDField:  "directorId",
			ObjLabel: "director",
			Metrics: []MetricDef{
				{Key: "PercentBusy", Name: "pmax_be_director_busy_percent"},
				{Key: "IOs", Name: "pmax_be_director_iops"},
				{Key: "MBReads", Name: "pmax_be_director_read_megabytes_per_second"},
				{Key: "MBWritten", Name: "pmax_be_director_write_megabytes_per_second"},
			},
		},
		{
			Category: "RDFDirector",
			IDField:  "directorId",
			ObjLabel: "director",
			Metrics: []MetricDef{
				{Key: "PercentBusy", Name: "pmax_rdf_director_busy_percent"},
				{Key: "IOs", Name: "pmax_rdf_director_iops"},
				{Key: "MBSentAndReceived", Name: "pmax_rdf_director_megabytes_per_second"},
			},
		},
		{
			Category: "StorageGroup",
			IDField:  "storageGroupId",
			ObjLabel: "storage_group",
			Metrics: []MetricDef{
				{Key: "HostIOs", Name: "pmax_storage_group_host_iops"},
				{Key: "HostReads", Name: "pmax_storage_group_host_read_iops"},
				{Key: "HostWrites", Name: "pmax_storage_group_host_write_iops"},
				{Key: "HostMBs", Name: "pmax_storage_group_host_megabytes_per_second"},
				{Key: "ReadResponseTime", Name: "pmax_storage_group_read_response_time_milliseconds"},
				{Key: "WriteResponseTime", Name: "pmax_storage_group_write_response_time_milliseconds"},
				{Key: "AllocatedCapacity", Name: "pmax_storage_group_allocated_capacity_gigabytes"},
			},
		},
		{
			Category: "SRP",
			IDField:  "srpId",
			ObjLabel: "srp",
			Metrics: []MetricDef{
				{Key: "HostIOs", Name: "pmax_srp_host_iops"},
				{Key: "HostMBs", Name: "pmax_srp_host_megabytes_per_second"},
				{Key: "ResponseTime", Name: "pmax_srp_response_time_milliseconds"},
			},
		},
	}
}
