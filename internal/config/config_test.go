package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInterpolatesEnvAndDefaults(t *testing.T) {
	t.Setenv("PMAX01_PASSWORD", "s3cret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server: {host: "0.0.0.0", port: "9104", uri: "/metrics"}
collection: {interval: "5m", timeout: "120s"}
servers:
  - {name: uni01, host: uni01.example.com, username: u, password: "${PMAX01_PASSWORD}", insecureSkipVerify: true}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Servers[0].Password != "s3cret" {
		t.Fatalf("password = %q, want s3cret", cfg.Servers[0].Password)
	}
	if cfg.Servers[0].BaseURL() != "https://uni01.example.com:8443" {
		t.Fatalf("BaseURL = %q, want :8443 default", cfg.Servers[0].BaseURL())
	}
	if cfg.Servers[0].APIVersion != "100" {
		t.Fatalf("apiVersion default = %q, want 100", cfg.Servers[0].APIVersion)
	}
	if cfg.Collection.MaxConcurrent != 8 {
		t.Fatalf("maxConcurrent default = %d, want 8", cfg.Collection.MaxConcurrent)
	}
}

func TestLoadDefaultsIntervalToPerfGranularity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	_ = os.WriteFile(path, []byte("servers:\n  - {name: uni01, host: h, username: u, password: p}\n"), 0o600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Collection.Interval.String() != "5m0s" {
		t.Fatalf("interval default = %s, want 5m0s (diagnostic perf granularity)", cfg.Collection.Interval)
	}
	if cfg.Server.Port != "9104" {
		t.Fatalf("port default = %s, want 9104", cfg.Server.Port)
	}
}

func TestLoadKeepsArraysAllowlist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	yaml := `
servers:
  - name: uni01
    host: h
    username: u
    password: p
    arrays: ["000297900046", "000297900047"]
`
	_ = os.WriteFile(path, []byte(yaml), 0o600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Servers[0].Arrays) != 2 || cfg.Servers[0].Arrays[0] != "000297900046" {
		t.Fatalf("arrays = %v, want the two configured symmetrix IDs", cfg.Servers[0].Arrays)
	}
}

func TestLoadRejectsEmptyServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	_ = os.WriteFile(path, []byte("servers: []\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when no servers configured")
	}
}

func TestLoadFailsOnUnsetEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	yaml := `
servers:
  - {name: uni01, host: h, username: u, password: "${PMAX_NOPE_UNSET}"}
`
	_ = os.WriteFile(path, []byte(yaml), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unset env var reference")
	}
}

func TestLoadInterpolatesHostAndUsername(t *testing.T) {
	t.Setenv("PMAX01_HOSTNAME", "uni-from-env.example.com")
	t.Setenv("PMAX01_USERNAME", "env-monitor")
	t.Setenv("PMAX01_PASSWORD", "env-secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
servers:
  - name: uni01
    host: "${PMAX01_HOSTNAME}"
    username: "${PMAX01_USERNAME}"
    password: "${PMAX01_PASSWORD}"
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s := cfg.Servers[0]
	if s.Host != "uni-from-env.example.com" {
		t.Fatalf("host = %q, want uni-from-env.example.com", s.Host)
	}
	if s.Username != "env-monitor" {
		t.Fatalf("username = %q, want env-monitor", s.Username)
	}
	if s.Password != "env-secret" {
		t.Fatalf("password = %q, want env-secret", s.Password)
	}
}

func TestLoadFailsOnUnsetHostEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	yaml := `
servers:
  - {name: uni01, host: "${PMAX_HOST_UNSET}", username: u, password: p}
`
	_ = os.WriteFile(path, []byte(yaml), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unset host env var reference")
	}
}

func TestLoadFailsOnUnsetUsernameEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	yaml := `
servers:
  - {name: uni01, host: h, username: "${PMAX_USER_UNSET}", password: p}
`
	_ = os.WriteFile(path, []byte(yaml), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unset username env var reference")
	}
}

func TestLoadReadsPasswordFile(t *testing.T) {
	dir := t.TempDir()
	pwPath := filepath.Join(dir, "secret")
	if err := os.WriteFile(pwPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "c.yaml")
	yaml := `
servers:
  - {name: uni01, host: h, username: u, passwordFile: "` + pwPath + `"}
`
	_ = os.WriteFile(path, []byte(yaml), 0o600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Servers[0].Password != "file-secret" {
		t.Fatalf("password = %q, want file-secret (trimmed)", cfg.Servers[0].Password)
	}
}
