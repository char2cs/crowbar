package agent

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Descriptor holds only fields the engine consumes. Every field here is
// load-bearing; provider-specific shapes (hook-config layout, native event
// names) live in the descriptor's literal write-side content, never here.
type Descriptor struct {
	ID    string `yaml:"id"`
	Spawn struct {
		Cmd                 string   `yaml:"cmd"`
		InteractiveRequired bool     `yaml:"interactive_required"`
		ForbidFlags         []string `yaml:"forbid_flags"`
		Args                []string `yaml:"args"`
		Env                 struct {
			Clear []string `yaml:"clear"`
		} `yaml:"env"`
	} `yaml:"spawn"`
	Session struct {
		Resume *ArgSpec `yaml:"resume"`
	} `yaml:"session"`
	ConfigInjection []InjectStep `yaml:"config_injection"`
	Hooks           HookSpec     `yaml:"hooks"`
	HandoffInject   []InjectStep `yaml:"handoff_inject"`
	// SystemPromptInject renders a true per-invocation system-prompt document
	// (e.g. the chat-title instruction) — distinct from HandoffInject, which
	// for some providers (codex) is a positional arg that would otherwise
	// hijack the CLI's initial user turn. See spawnSegment's injectTitle
	// branch, the only caller.
	SystemPromptInject []InjectStep `yaml:"system_prompt_inject"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}

// HookSpec is the read side: how to parse this CLI's hook payloads (format) and
// where each Crowbar vocabulary field sits inside each canonical event's
// payload (events[canonical][vocab] = dotted path).
type HookSpec struct {
	Format string                       `yaml:"format"`
	Events map[string]map[string]string `yaml:"events"`
}

// InjectStep is one declarative injection verb, e.g. `- pass_arg: {arg: --settings, value: x}`.
// The YAML is a single-key map; the key is the verb, the value is its args.
type InjectStep struct {
	Verb string
	Args map[string]any
}

func (s *InjectStep) UnmarshalYAML(value *yaml.Node) error {
	var m map[string]map[string]any
	if err := value.Decode(&m); err != nil {
		return fmt.Errorf("agent: inject step decode: %w", err)
	}
	if len(m) != 1 {
		return fmt.Errorf("agent: inject step must have exactly one verb, got %d", len(m))
	}
	for verb, args := range m {
		s.Verb = verb
		s.Args = args
	}
	return nil
}

func LoadDescriptor(data []byte) (*Descriptor, error) {
	var d Descriptor
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("agent: descriptor unmarshal: %w", err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

func (d *Descriptor) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("agent: descriptor missing id")
	}
	if d.Spawn.Cmd == "" {
		return fmt.Errorf("agent: descriptor %q missing spawn.cmd", d.ID)
	}
	if !d.Spawn.InteractiveRequired {
		return fmt.Errorf("agent: descriptor %q must set spawn.interactive_required", d.ID)
	}
	if d.Hooks.Format == "" {
		return fmt.Errorf("agent: descriptor %q missing hooks.format", d.ID)
	}
	if ss := d.Hooks.Events["session_start"]; ss["session_id"] == "" {
		return fmt.Errorf("agent: descriptor %q hooks.events.session_start must map session_id", d.ID)
	}
	if ts := d.Hooks.Events["turn_stop"]; ts["message"] == "" {
		return fmt.Errorf("agent: descriptor %q hooks.events.turn_stop must map message", d.ID)
	}
	return nil
}

// ParsePayload decodes raw hook bytes into a map per the descriptor's declared
// format. An unsupported format is an explicit error (documented boundary).
func (d *Descriptor) ParsePayload(raw []byte) (map[string]any, error) {
	switch d.Hooks.Format {
	case "json":
		if len(raw) == 0 {
			return map[string]any{}, nil
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("agent: descriptor %q parse json payload: %w", d.ID, err)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("agent: descriptor %q unsupported hooks.format %q", d.ID, d.Hooks.Format)
	}
}
