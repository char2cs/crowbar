package agent

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Descriptor struct {
	ID      string `yaml:"id"`
	Version struct {
		Pinned      string `yaml:"pinned"`
		CompatCheck string `yaml:"compat_check"`
	} `yaml:"version"`
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
		Assign *ArgSpec `yaml:"assign"`
		Resume *ArgSpec `yaml:"resume"`
	} `yaml:"session"`
	ConfigInjection []InjectStep       `yaml:"config_injection"`
	Hooks           map[string]HookMap `yaml:"hooks"`
	Transcript      struct {
		FromHook string `yaml:"from_hook"`
		Locate   string `yaml:"locate"`
		Content  string `yaml:"content"`
	} `yaml:"transcript"`
	HandoffInject []InjectStep `yaml:"handoff_inject"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}

type HookMap struct {
	ProviderEvent string            `yaml:"provider_event"`
	Fields        map[string]string `yaml:"fields"`
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
	if _, ok := d.Hooks["session_start"]; !ok {
		return fmt.Errorf("agent: descriptor %q missing hooks.session_start", d.ID)
	}
	if d.Hooks["session_start"].Fields["session_id"] == "" {
		return fmt.Errorf("agent: descriptor %q session_start must map session_id", d.ID)
	}
	return nil
}
