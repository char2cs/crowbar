// Package config loads ~/.crowbar/config.yaml over embedded defaults and
// exposes a singleton Config via Get(). The singleton is initialized once
// via sync.Once; fields absent from the user file retain their embedded defaults.
package config

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"sync"

	yaml "gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

var (
	//go:embed config.yaml
	defaultConfigBytes []byte

	cfg  *Config
	once sync.Once
)

// IntelligenceConfig maps intelligence tiers to model names.
type IntelligenceConfig struct {
	Low    string `yaml:"low"`
	Medium string `yaml:"medium"`
	High   string `yaml:"high"`
}

// APIConfig holds API transport settings.
type APIConfig struct {
	Host string `yaml:"host"`
}

// Config is the top-level application configuration.
type Config struct {
	Intelligence IntelligenceConfig `yaml:"intelligence"`
	API          APIConfig          `yaml:"api"`
}

// Get returns the singleton Config. It starts from the embedded defaults and
// overlays any fields present in ~/.crowbar/config.yaml. Fields absent from
// the user file keep their embedded default values.
func Get() Config {
	once.Do(func() {
		cfg = getDefault()

		configPath := filepath.Join(metadata.GetHome(), "config.yaml")
		data, err := os.ReadFile(configPath)
		if err != nil {
			// No user config file — use defaults as-is.
			return
		}

		// Overlay user config onto defaults; ignore parse errors to stay safe.
		if err := yaml.Unmarshal(data, cfg); err != nil {
			cfg = getDefault()
		}
	})
	return *cfg
}

// GetIntelligence returns the intelligence tier configuration.
func GetIntelligence() IntelligenceConfig {
	return Get().Intelligence
}

// GetAPI returns the API transport configuration.
func GetAPI() APIConfig {
	return Get().API
}

// ModelFor maps a tier name ("low", "medium", "high") to its model name string.
// Returns an empty string for unknown tier names.
func (c IntelligenceConfig) ModelFor(level string) string {
	switch strings.ToLower(level) {
	case "low":
		return c.Low
	case "medium":
		return c.Medium
	case "high":
		return c.High
	default:
		return ""
	}
}

func getDefault() *Config {
	c := &Config{}
	if err := yaml.Unmarshal(defaultConfigBytes, c); err != nil {
		// The embedded config.yaml is baked in at build time. A parse failure
		// means the binary is corrupt — there is no safe fallback.
		panic("config: failed to parse embedded config.yaml: " + err.Error())
	}
	return c
}

// resetForTesting resets the singleton so tests can re-initialize it.
func resetForTesting() {
	cfg = nil
	once = sync.Once{}
}
