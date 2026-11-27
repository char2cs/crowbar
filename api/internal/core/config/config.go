package config

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"

	yaml "gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

var (
	//go:embed default.yaml
	defaultConfigByte []byte
	config            *Config
	once              sync.Once
)

type Prompts struct {
	CapabilitiesInstruction string `yaml:"capabilities_instruction"`
	ThreadLineage           string `yaml:"thread_lineage"`
	HandoffWrapper          string `yaml:"handoff_wrapper"`
	HandoffResumeWrapper    string `yaml:"handoff_resume_wrapper"`
	HandoffPointer          string `yaml:"handoff_pointer"`
}

type ConfigData struct {
	Prompts Prompts `yaml:"prompts"`
}

type Config struct {
	Config ConfigData `yaml:"config"`
}

func Get() *Config {
	once.Do(func() {
		config = getDefaultConfig()
		h, err := home.Resolve()
		if err != nil {
			return
		}
		configBytes, err := os.ReadFile(filepath.Clean(h.Config()))
		if err != nil {
			return
		}
		if err := yaml.Unmarshal(configBytes, config); err != nil {
			config = getDefaultConfig()
		}
	})
	return config
}

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
