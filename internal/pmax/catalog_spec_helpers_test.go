package pmax

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

const (
	spec104Path = "../../docs/swagger/12315-10.4.0.json"
	spec92Path  = "../../docs/swagger/openapi-9.2.json"
)

var enumRe = regexp.MustCompile(`\*\s+\*\*([A-Za-z0-9_]+)\*\*`)

// specMetrics returns category -> set of exact-case Unisphere metric names, read
// from each /performance/{Cat}/metrics POST requestBody $ref param schema. Casing
// of the param schema name is inconsistent, so we follow the $ref, never guess it.
func specMetrics(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec %s: %v", path, err)
	}
	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec %s: %v", path, err)
	}
	out := map[string]map[string]bool{}
	prefix, suffix := "/performance/", "/metrics"
	for p, methods := range doc.Paths {
		if len(p) <= len(prefix)+len(suffix) ||
			p[:len(prefix)] != prefix || p[len(p)-len(suffix):] != suffix {
			continue
		}
		cat := p[len(prefix) : len(p)-len(suffix)]
		if cat == "" || containsByte(cat, '/') { // skip help/{...} sub-paths
			continue
		}
		ref := metricsParamRef(methods["post"])
		if ref == "" {
			continue
		}
		desc := metricsEnumDesc(doc.Components.Schemas[ref])
		set := map[string]bool{}
		for _, m := range enumRe.FindAllStringSubmatch(desc, -1) {
			set[m[1]] = true
		}
		if len(set) > 0 {
			out[cat] = set
		}
	}
	return out
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

// metricsParamRef pulls the schema name out of post.requestBody.content.
// application/json.schema.$ref (e.g. "#/components/schemas/storageGroupParam").
func metricsParamRef(post json.RawMessage) string {
	var op struct {
		RequestBody struct {
			Content struct {
				JSON struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"application/json"`
			} `json:"content"`
		} `json:"requestBody"`
	}
	if json.Unmarshal(post, &op) != nil {
		return ""
	}
	ref := op.RequestBody.Content.JSON.Schema.Ref
	if i := lastSlash(ref); i >= 0 {
		return ref[i+1:]
	}
	return ""
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// metricsEnumDesc returns the `metrics` property description that carries the
// `* **Name** - text` enumeration (sometimes under items, sometimes direct).
func metricsEnumDesc(schema json.RawMessage) string {
	var s struct {
		Properties struct {
			Metrics struct {
				Description string `json:"description"`
				Items       struct {
					Description string `json:"description"`
				} `json:"items"`
			} `json:"metrics"`
		} `json:"properties"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return ""
	}
	if s.Properties.Metrics.Items.Description != "" {
		return s.Properties.Metrics.Items.Description
	}
	return s.Properties.Metrics.Description
}

func TestSpecMetricsLoads(t *testing.T) {
	got := specMetrics(t, spec104Path)
	want := map[string]int{
		"Array": 135, "FEDirector": 105, "BEDirector": 31, "FEPort": 33,
		"BEPort": 10, "CachePartition": 47, "RDFDirector": 34,
		"StorageGroup": 184, "SRP": 56,
	}
	for cat, n := range want {
		if len(got[cat]) != n {
			t.Errorf("category %s: got %d metrics, want %d", cat, len(got[cat]), n)
		}
	}
	if !got["StorageGroup"]["HostIOs"] {
		t.Errorf("StorageGroup enum missing known key HostIOs")
	}
}
