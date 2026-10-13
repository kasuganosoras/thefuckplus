package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	ProviderOpenAI   = "openai"
	ProviderDeepSeek = "deepseek"
	ProviderCustom   = "custom"

	ExecuteConfirm = "confirm"
	ExecuteExec    = "execute"
)

var (
	ErrNotConfigured = errors.New("need_config")
)

type AgentConfig struct {
	Enabled        bool     `yaml:"enabled"`
	MaxRounds      int      `yaml:"max_rounds"`
	MaxFileBytes   int      `yaml:"max_file_bytes"`
	MaxListEntries int      `yaml:"max_list_entries"`
	AllowSensitive bool     `yaml:"allow_sensitive"`
	ExtraAllow     []string `yaml:"extra_allow"`
	ExtraDeny      []string `yaml:"extra_deny"`
}

type Config struct {
	Provider      string      `yaml:"provider"`
	APIKey        string      `yaml:"api_key"`
	BaseURL       string      `yaml:"base_url"`
	Model         string      `yaml:"model"`
	ExecuteMode   string      `yaml:"execute_mode"`
	MaxCandidates int         `yaml:"max_candidates"`
	TimeoutSec    int         `yaml:"timeout_sec"`
	Agent         AgentConfig `yaml:"agent"`
}

func DefaultAgent() AgentConfig {
	return AgentConfig{
		Enabled:        true,
		MaxRounds:      4,
		MaxFileBytes:   32768,
		MaxListEntries: 100,
		AllowSensitive: false,
		ExtraAllow:     nil,
		ExtraDeny:      nil,
	}
}

func Default() Config {
	return Config{
		Provider:      ProviderOpenAI,
		BaseURL:       "https://api.openai.com/v1",
		Model:         "gpt-4o-mini",
		ExecuteMode:   ExecuteConfirm,
		MaxCandidates: 5,
		TimeoutSec:    30,
		Agent:         DefaultAgent(),
	}
}

func Preset(provider string) Config {
	cfg := Default()
	cfg.Provider = provider
	switch provider {
	case ProviderDeepSeek:
		cfg.BaseURL = "https://api.deepseek.com/v1"
		cfg.Model = "deepseek-chat"
	case ProviderCustom:
		cfg.BaseURL = "http://localhost:11434/v1"
		cfg.Model = "llama3.2"
	default:
		cfg.Provider = ProviderOpenAI
		cfg.BaseURL = "https://api.openai.com/v1"
		cfg.Model = "gpt-4o-mini"
	}
	return cfg
}

func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "thefuckplus"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "thefuckplus"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, ErrNotConfigured
		}
		return Config{}, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if !cfg.IsConfigured() {
		return cfg, ErrNotConfigured
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.Provider == "" {
		c.Provider = d.Provider
	}
	if c.BaseURL == "" {
		p := Preset(c.Provider)
		c.BaseURL = p.BaseURL
	}
	if c.Model == "" {
		p := Preset(c.Provider)
		c.Model = p.Model
	}
	if c.ExecuteMode == "" {
		c.ExecuteMode = d.ExecuteMode
	}
	if c.ExecuteMode != ExecuteConfirm && c.ExecuteMode != ExecuteExec {
		c.ExecuteMode = ExecuteConfirm
	}
	if c.MaxCandidates <= 0 {
		c.MaxCandidates = d.MaxCandidates
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = d.TimeoutSec
	}
	c.Agent.applyDefaults()
}

func (a *AgentConfig) applyDefaults() {
	d := DefaultAgent()
	if a.MaxRounds <= 0 {
		a.MaxRounds = d.MaxRounds
	}
	if a.MaxFileBytes <= 0 {
		a.MaxFileBytes = d.MaxFileBytes
	}
	if a.MaxListEntries <= 0 {
		a.MaxListEntries = d.MaxListEntries
	}
	// Enabled: Load() starts from Default() (true). Omitted agent key keeps it.
	// Explicit `enabled: false` in YAML disables. ExtraAllow/ExtraDeny stay as-is.
}

func (c Config) IsConfigured() bool {
	if c.BaseURL == "" || c.Model == "" {
		return false
	}
	// Local OpenAI-compatible servers may not require a key.
	if c.Provider == ProviderCustom && c.APIKey == "" {
		return true
	}
	return c.APIKey != ""
}

// HTTPTimeout returns the HTTP client timeout, scaled for agent rounds.
func (c Config) HTTPTimeout() int {
	sec := c.TimeoutSec
	if sec <= 0 {
		sec = 30
	}
	if !c.Agent.Enabled {
		return sec
	}
	rounds := c.Agent.MaxRounds
	if rounds <= 0 {
		rounds = 4
	}
	total := sec * (rounds + 1)
	const capSec = 300
	if total > capSec {
		return capSec
	}
	return total
}

func Save(cfg Config) error {
	cfg.applyDefaults()
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
