package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSetsUnsetVars(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DOTENV_TEST_HOST=h1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_HOST", "") // register cleanup, then unset for real
	_ = os.Unsetenv("DOTENV_TEST_HOST")

	LoadDotEnv(cfg)
	if got := os.Getenv("DOTENV_TEST_HOST"); got != "h1" {
		t.Errorf("DOTENV_TEST_HOST = %q, want h1", got)
	}
}

func TestLoadDotEnvNeverOverridesRealEnv(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DOTENV_TEST_PW=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_PW", "from-env")

	LoadDotEnv(cfg)
	if got := os.Getenv("DOTENV_TEST_PW"); got != "from-env" {
		t.Errorf("DOTENV_TEST_PW = %q, want from-env (real env must win)", got)
	}
}

func TestLoadDotEnvMissingFileIsNoop(t *testing.T) {
	LoadDotEnv(filepath.Join(t.TempDir(), "config.yaml")) // must not panic or log fatal
}

func TestLoadDotEnvFeedsInterpolation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("PMAX_DOTENV_HOST=uni.example.com\nPMAX_DOTENV_USER=mon\nPMAX_DOTENV_PW=s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`
servers:
  - name: s1
    host: "${PMAX_DOTENV_HOST}"
    username: "${PMAX_DOTENV_USER}"
    password: "${PMAX_DOTENV_PW}"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"PMAX_DOTENV_HOST", "PMAX_DOTENV_USER", "PMAX_DOTENV_PW"} {
		t.Setenv(v, "")
		_ = os.Unsetenv(v)
	}

	LoadDotEnv(cfgPath)
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Servers[0]
	if s.Host != "uni.example.com" || s.Username != "mon" || s.Password != "s3cret" {
		t.Errorf("interpolated server = %+v", s)
	}
}
