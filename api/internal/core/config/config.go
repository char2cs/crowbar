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

// Prompts holds Crowbar's agent-facing prompt templates. Placeholders
// ({crowbar}, {chatid}, {conversation}) are expanded by Crowbar at injection time.
type Prompts struct {
	TitleInstruction string `yaml:"title_instruction"`
	// HandoffWrapper wraps the WHOLE conversation for a provider joining the chat
	// fresh (it has no session of its own to resume, so it has no history).
	HandoffWrapper string `yaml:"handoff_wrapper"`
	// HandoffResumeWrapper wraps only the GAP for a provider resumed into its own
	// native session — it already holds everything up to the moment it was
	// switched out, so it is handed just what happened while it was away.
	HandoffResumeWrapper string `yaml:"handoff_resume_wrapper"`
}

// ConfigData is the top-level config section.
type ConfigData struct {
	Prompts Prompts `yaml:"prompts"`
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

// GetPrompts returns the configured agent-facing prompt templates.
func GetPrompts() Prompts {
	return Get().Config.Prompts
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
