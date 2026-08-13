// Package config loads and validates the exporter configuration.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// EnvBool is a boolean config value that may be written in YAML either as a
// native boolean (insecureSkipVerify: true) or as a ${VAR} environment
// reference resolved at secret-resolution time (insecureSkipVerify:
// ${PMAX1_SKIP_CERTIFICATE}). Backward compatible with existing native-bool
// configs; an omitted field defaults to false.
type EnvBool struct {
	raw string // ${...} reference or literal string form, when written as a string
	val bool   // resolved value
}

// NewEnvBool returns an already-resolved EnvBool (for tests / programmatic config).
func NewEnvBool(v bool) EnvBool { return EnvBool{val: v} }

// Bool returns the resolved boolean value.
func (b EnvBool) Bool() bool { return b.val }

// UnmarshalYAML accepts either a native YAML boolean or a string (which may
// be a ${VAR} reference resolved later by Resolve).
func (b *EnvBool) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var bv bool
	if err := unmarshal(&bv); err == nil {
		b.val = bv
		return nil
	}
	var s string
	if err := unmarshal(&s); err != nil {
		return fmt.Errorf("must be a boolean or ${ENV} reference: %w", err)
	}
	b.raw = s
	return nil
}

// Resolve expands any ${VAR} reference (via expand) and parses the result to
// a bool. No-op when the value was a native boolean or omitted. Empty
// expansion => false; non-boolean expansion is an error.
func (b *EnvBool) Resolve(expand func(string) (string, error)) error {
	if b.raw == "" {
		return nil
	}
	s, err := expand(b.raw)
	if err != nil {
		return err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		b.val = false
		return nil
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("cannot parse %q as boolean", s)
	}
	b.val = v
	return nil
}

// Server is one Unisphere for PowerMax instance to monitor. A single Unisphere
// can manage several arrays; Arrays optionally restricts collection to the
// listed symmetrix IDs (empty = every performance-registered local array).
type Server struct {
	Name               string   `yaml:"name"`
	Host               string   `yaml:"host"`
	Port               int      `yaml:"port"` // defaults to 8443
	Username           string   `yaml:"username"`
	Password           string   `yaml:"password"`
	PasswordFile       string   `yaml:"passwordFile"`
	InsecureSkipVerify EnvBool  `yaml:"insecureSkipVerify"`
	APIVersion         string   `yaml:"apiVersion"` // Unisphere REST version prefix, defaults to "100"
	Arrays             []string `yaml:"arrays"`
}

// BaseURL returns the https://host:port root for the Unisphere REST API.
func (s Server) BaseURL() string {
	port := s.Port
	if port == 0 {
		port = 8443
	}
	return fmt.Sprintf("https://%s:%d", s.Host, port)
}

// ServerHTTP holds the exporter's own HTTP-server settings. Named to avoid colliding
// with the Unisphere target Server struct.
type ServerHTTP struct {
	Host    string `yaml:"host"`
	Port    string `yaml:"port"`
	URI     string `yaml:"uri"`
	LogName string `yaml:"logName"`
}

// Collection holds loop timing. MaxConcurrent caps in-flight performance queries
// per Unisphere instance (object-level metrics are one POST per object).
// VolumeMetrics opts into per-volume performance (high cardinality — one series
// set per device); VolumeStorageGroups optionally restricts it to listed SGs.
type Collection struct {
	Interval            time.Duration `yaml:"interval"`
	Timeout             time.Duration `yaml:"timeout"`
	MaxConcurrent       int           `yaml:"maxConcurrent"`
	VolumeMetrics       bool          `yaml:"volumeMetrics"`
	VolumeInventory     bool          `yaml:"volumeInventory"`
	VolumeStorageGroups []string      `yaml:"volumeStorageGroups"`
}

// OTel configures optional OTLP metric export.
type OTel struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
	Insecure bool   `yaml:"insecure"`
	Interval string `yaml:"interval"`
}

// Config is the whole file.
type Config struct {
	Server     ServerHTTP `yaml:"server"`
	Collection Collection `yaml:"collection"`
	OTel       OTel       `yaml:"otel"`
	Servers    []Server   `yaml:"servers"`
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-[^}]*)?\}`)

// interpolate replaces every ${VAR} in s with its environment value, returning an
// error if any referenced variable is unset. Failing fast turns a typo'd secret
// name into a config-load error instead of repeated runtime auth failures.
//
// A reference may carry a fallback as ${VAR:-default}, borrowing the shell /
// docker-compose syntax and its meaning: unset OR empty falls back, and the reference
// never errors. That lets a shipped config.yaml drive a non-secret setting from the
// environment while still starting on a host that never exported it. Use it only where a
// safe default exists.
//
// A bare ${VAR} fails when the variable is UNSET; an exported-but-empty one expands to
// the empty string, as it always has. Credential fields get the stricter treatment —
// see interpolateSecret.
func interpolate(s string) (string, error) {
	var missing []string
	out := envRef.ReplaceAllStringFunc(s, func(m string) string {
		sub := envRef.FindStringSubmatch(m)
		name, fallback := sub[1], sub[2]
		v, ok := os.LookupEnv(name)
		if ok && v != "" {
			return v
		}
		if fallback != "" {
			return fallback[len(":-"):] // group 2 keeps its ":-" prefix, so "" means absent
		}
		if !ok {
			missing = append(missing, name)
		}
		return ""
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unset environment variable(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// interpolateSecret expands like interpolate, but additionally rejects a credential that was
// written as an env reference yet resolves to nothing. A stray `PMAX1_PASSWORD=` line in
// a .env file is a plausible typo, and without this the exporter would authenticate with an
// empty credential and report a failure that names the wrong cause.
//
// It fires only when the field actually contains a ${...} reference: a literal value is
// passed through untouched and an omitted optional credential stays omitted, so it cannot
// break a config that never referenced the environment in the first place.
func interpolateSecret(field, s string) (string, error) {
	out, err := interpolate(s)
	if err != nil {
		return "", err
	}
	// The message names the FIELD and nothing else. Config-load errors are logged, and
	// every part of s is potentially secret — even the variable name is a substring of a
	// credential field, which is why it is not echoed either. The offending reference is
	// visible in config.yaml next to the field this names.
	if out == "" && envRef.MatchString(s) {
		return "", fmt.Errorf("%s is set from an environment reference that resolved to an "+
			"empty value; export the variable or remove the reference", field)
	}
	return out, nil
}

// Load reads, interpolates ${ENV} references, applies defaults, and validates.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		host, err := interpolateSecret("host", s.Host)
		if err != nil {
			return nil, fmt.Errorf("server %s host: %w", s.Name, err)
		}
		s.Host = host
		username, err := interpolateSecret("username", s.Username)
		if err != nil {
			return nil, fmt.Errorf("server %s username: %w", s.Name, err)
		}
		s.Username = username
		pw, err := interpolateSecret("password", s.Password)
		if err != nil {
			return nil, fmt.Errorf("server %s password: %w", s.Name, err)
		}
		s.Password = pw
		if s.PasswordFile != "" && s.Password == "" {
			b, err := os.ReadFile(s.PasswordFile)
			if err != nil {
				return nil, fmt.Errorf("server %s passwordFile: %w", s.Name, err)
			}
			s.Password = strings.TrimSpace(string(b))
		}
		if err := s.InsecureSkipVerify.Resolve(interpolate); err != nil {
			return nil, fmt.Errorf("server %s insecureSkipVerify: %w", s.Name, err)
		}
		if s.APIVersion == "" {
			s.APIVersion = "100"
		}
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "9443"
	}
	if cfg.Server.URI == "" {
		cfg.Server.URI = "/metrics"
	}
	if cfg.Collection.Interval == 0 {
		// Unisphere diagnostic-level performance data has 5-minute granularity;
		// polling faster only re-reads the same datapoint.
		cfg.Collection.Interval = 5 * time.Minute
	}
	if cfg.Collection.Timeout == 0 {
		cfg.Collection.Timeout = 120 * time.Second
	}
	if cfg.Collection.MaxConcurrent == 0 {
		cfg.Collection.MaxConcurrent = 8
	}
	if len(cfg.Servers) == 0 {
		return nil, fmt.Errorf("no servers configured")
	}
	return &cfg, nil
}
