package config

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"

	yaml "gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

var (
	//go:embed default.yaml
	defaultConfigByte []byte
	config            *Config
	once              sync.Once
)

// Intelligence holds the intelligence-tier → model-id mapping.
type Intelligence struct {
	Light  string `yaml:"light"`
	Medium string `yaml:"medium"`
	Heavy  string `yaml:"heavy"`
}

// ConfigData is the top-level config section.
type ConfigData struct {
	Intelligence Intelligence `yaml:"intelligence"`
}

// Config is the full config structure loaded from default.yaml and overlaid by
// the user's ~/.crowbar/config.yaml.
type Config struct {
	Config ConfigData `yaml:"config"`
}

// Get returns the singleton config: embedded defaults overlaid by the user's
// config.yaml at metadata.GetConfigPath(). Absent user fields keep defaults.
func Get() *Config {
	once.Do(func() {
		config = getDefaultConfig()
		configBytes, err := os.ReadFile(filepath.Clean(metadata.GetConfigPath()))
		if err != nil {
			return
		}
		if err := yaml.Unmarshal(configBytes, config); err != nil {
			config = getDefaultConfig()
		}
	})
	return config
}

// GetIntelligence returns the configured intelligence-tier → model mapping.
func GetIntelligence() Intelligence {
	return Get().Config.Intelligence
}

// ModelForTier maps an intelligence tier name to its model id, "" if unknown.
func ModelForTier(
	tier string,
) string {
	i := GetIntelligence()
	switch tier {
	case "light":
		return i.Light
	case "medium":
		return i.Medium
	case "heavy":
		return i.Heavy
	default:
		return ""
	}
}

func getDefaultConfig() *Config {
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultConfigByte, cfg); err != nil {
		panic("config: failed to parse embedded default.yaml: " + err.Error())
	}
	return cfg
}

func resetForTesting() {
	config = nil
	once = sync.Once{}
}
