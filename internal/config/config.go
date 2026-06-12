// Package config loads and validates the exporter configuration.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

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
	InsecureSkipVerify bool     `yaml:"insecureSkipVerify"`
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
type Collection struct {
	Interval      time.Duration `yaml:"interval"`
	Timeout       time.Duration `yaml:"timeout"`
	MaxConcurrent int           `yaml:"maxConcurrent"`
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

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// interpolate replaces every ${VAR} in s with its environment value, returning an
// error if any referenced variable is unset. Failing fast turns a typo'd secret
// name into a config-load error instead of repeated runtime auth failures.
func interpolate(s string) (string, error) {
	var missing []string
	out := envRef.ReplaceAllStringFunc(s, func(m string) string {
		name := envRef.FindStringSubmatch(m)[1]
		v, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unset environment variable(s): %s", strings.Join(missing, ", "))
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
		host, err := interpolate(s.Host)
		if err != nil {
			return nil, fmt.Errorf("server %s host: %w", s.Name, err)
		}
		s.Host = host
		username, err := interpolate(s.Username)
		if err != nil {
			return nil, fmt.Errorf("server %s username: %w", s.Name, err)
		}
		s.Username = username
		pw, err := interpolate(s.Password)
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
		if s.APIVersion == "" {
			s.APIVersion = "100"
		}
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "9104"
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
