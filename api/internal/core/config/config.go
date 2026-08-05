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
// ({conversation}, {ledger_dir}, {ledger_cut}) are expanded by Crowbar at
// injection time.
//
// Every prompt here is CONTEXT, never a capability: a prompt can only ask a model
// to do something, and a model that ignores it leaves no trace. Anything Crowbar
// needs an agent to be able to DO is a tool on the MCP surface instead — which is
// why the shell titling instruction that used to live here is gone.
type Prompts struct {
	// CapabilitiesInstruction tells a model that Crowbar's tools exist and, more
	// importantly, WHEN to prefer them. Tool descriptions alone do not override a
	// model's prior — asked to "review this branch" it reaches for gh or writes
	// prose — so this is a directive, not a capability list.
	//
	// It must never name a capability that is not registered. A directive pointing at
	// an absent tool family, while forbidding the fallback the model would otherwise
	// reach for, is worse than no directive at all — so the review-tools directive
	// was held back until the review tools it names actually existed (Task 13).
	CapabilitiesInstruction string `yaml:"capabilities_instruction"`
	// HandoffWrapper wraps the WHOLE conversation for a provider joining the chat
	// fresh (it has no session of its own to resume, so it has no history).
	HandoffWrapper string `yaml:"handoff_wrapper"`
	// HandoffResumeWrapper wraps only the GAP for a provider resumed into its own
	// native session — it already holds everything up to the moment it was
	// switched out, so it is handed just what happened while it was away.
	HandoffResumeWrapper string `yaml:"handoff_resume_wrapper"`
	// HandoffPointer is the SHORT message handed to a provider that can only be
	// reached through a USER MESSAGE (a resumed codex ignores every config channel).
	// It POINTS at the conversation ledger already on disk ({ledger_dir}) and says
	// where to start reading ({ledger_cut}) — it never carries the transcript
	// itself, which would dump the whole handed-off exchange into the chat.
	HandoffPointer string `yaml:"handoff_pointer"`
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
