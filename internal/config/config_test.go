package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHTTPTimeoutScalesWithAgent(t *testing.T) {
	cfg := Default()
	cfg.TimeoutSec = 30
	cfg.Agent.MaxRounds = 4
	cfg.Agent.Enabled = true
	got := cfg.HTTPTimeout()
	if got != 150 { // 30 * (4+1)
		t.Fatalf("got %d", got)
	}
	cfg.Agent.Enabled = false
	if cfg.HTTPTimeout() != 30 {
		t.Fatalf("disabled should not scale")
	}
}

func TestLoadPreservesAgentEnabledWhenOmitted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, "thefuckplus")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`provider: custom
api_key: ""
base_url: http://localhost:9/v1
model: test
execute_mode: confirm
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Agent.Enabled {
		t.Fatal("omitted agent block should keep default enabled=true")
	}
	if cfg.Agent.MaxRounds != 4 {
		t.Fatalf("max_rounds default: %d", cfg.Agent.MaxRounds)
	}
}

func TestLoadAgentDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, "thefuckplus")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`provider: custom
base_url: http://localhost:9/v1
model: test
agent:
  enabled: false
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.Enabled {
		t.Fatal("expected enabled false")
	}
	if cfg.Agent.MaxRounds != 4 {
		t.Fatalf("defaults for other fields: %d", cfg.Agent.MaxRounds)
	}
}

func TestAgentPartialYAML(t *testing.T) {
	cfg := Default()
	if err := yaml.Unmarshal([]byte("agent:\n  max_rounds: 2\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.applyDefaults()
	if cfg.Agent.MaxRounds != 2 {
		t.Fatalf("got %d", cfg.Agent.MaxRounds)
	}
	if !cfg.Agent.Enabled {
		t.Fatal("partial agent yaml should keep Enabled from Default")
	}
}
