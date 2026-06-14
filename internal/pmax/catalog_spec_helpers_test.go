package pmax

import (
	"encoding/json"
	"os"
	"path/filepath"
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

var pmaxNameRe = regexp.MustCompile(`pmax_[a-z0-9_]+`)

// emittedNames is every pmax_* metric the exporter can emit: the set of pmax_
// string literals in non-test Go source under internal/pmax. A name only reaches
// a dashboard if some collector emits a Sample with that literal, so this is the
// authoritative, drift-proof emitted set (no hand-maintained list).
func emittedNames(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	matches, err := filepath.Glob("*.go") // internal/pmax; widen if collectors move
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range matches {
		if len(f) > 8 && f[len(f)-8:] == "_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, n := range pmaxNameRe.FindAllString(string(b), -1) {
			set[n] = true
		}
	}
	return set
}

// dashboardRefs returns every pmax_* metric referenced across the dashboard JSONs.
func dashboardRefs(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("../../grafana/dashboards/*.json")
	if err != nil {
		t.Fatalf("glob dashboards: %v", err)
	}
	out := map[string][]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		seen := map[string]bool{}
		for _, n := range pmaxNameRe.FindAllString(string(b), -1) {
			if !seen[n] {
				seen[n] = true
				out[f] = append(out[f], n)
			}
		}
	}
	return out
}

// specResponseProps returns the property names of the 200 response schema for the
// first path whose key ends with pathSuffix (paths are version-prefixed, e.g.
// /104/sloprovisioning/.../volume/{volumeId}).
func specResponseProps(t *testing.T, path, pathSuffix string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var doc struct {
		Paths map[string]struct {
			Get struct {
				Responses struct {
					OK struct {
						Content struct {
							JSON struct {
								Schema struct {
									Ref string `json:"$ref"`
								} `json:"schema"`
							} `json:"application/json"`
						} `json:"content"`
					} `json:"200"`
				} `json:"responses"`
			} `json:"get"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	for p, item := range doc.Paths {
		if len(p) >= len(pathSuffix) && p[len(p)-len(pathSuffix):] == pathSuffix {
			name := item.Get.Responses.OK.Content.JSON.Schema.Ref
			if i := lastSlash(name); i >= 0 {
				name = name[i+1:]
			}
			props := map[string]bool{}
			for k := range doc.Components.Schemas[name].Properties {
				props[k] = true
			}
			return props
		}
	}
	t.Fatalf("no path ending %q in %s", pathSuffix, path)
	return nil
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
