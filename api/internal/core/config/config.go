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

// Prompts holds Crowbar's agent-facing prompt templates; placeholders are expanded at injection time.
// Every prompt is context, never a capability: a model can ignore a prompt without trace, so anything
// an agent must be able to do is an MCP tool instead.
type Prompts struct {
	// CapabilitiesInstruction tells a model when to prefer Crowbar's tools, since tool descriptions alone
	// do not override its prior. It must never name an unregistered capability: a directive pointing at an
	// absent tool while forbidding the fallback is worse than none.
	CapabilitiesInstruction string `yaml:"capabilities_instruction"`
	// ThreadLineage is the directive a thread spawns with: its ancestor chat ids ({lineage}, nearest first)
	// and an instruction to read them. It carries ids, never turns, so the thread reads its parent as it
	// stands; a chat with no chat ancestors is injected nothing.
	ThreadLineage string `yaml:"thread_lineage"`
	// HandoffWrapper wraps the conversation for a provider joining the chat fresh
	// (it has no session of its own to resume, so it has no history) — capped to
	// its first ask plus the most recent turns, not the whole ledger; see HandoffWindow.
	HandoffWrapper string `yaml:"handoff_wrapper"`
	// HandoffResumeWrapper wraps only the gap for a provider resumed into its own native session, which
	// already holds everything before it was switched out; the gap is capped by HandoffWindow.
	HandoffResumeWrapper string `yaml:"handoff_resume_wrapper"`
	// HandoffPointer is the short message for a provider reachable only through a user message: it points
	// at its own get_chat_log tool ({chat_id}, {gap_turns}) instead of dumping the transcript into the chat.
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
// config.yaml in the Crowbar home. Absent user fields keep defaults, and so
// does any failure to resolve or read the home.
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
