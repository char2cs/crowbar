package spec

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// InjectStep is one declarative injection verb, e.g.
// `- pass_arg: {arg: --settings, value: x}`. The YAML is a single-key map: the
// key is the verb, the value is its arguments.
type InjectStep struct {
	Verb string
	Args map[string]any
}

func (s *InjectStep) UnmarshalYAML(value *yaml.Node) error {
	var m map[string]map[string]any
	if err := value.Decode(&m); err != nil {
		return fmt.Errorf("spec: inject step decode: %w", err)
	}
	if len(m) != 1 {
		return fmt.Errorf("spec: inject step must have exactly one verb, got %d", len(m))
	}
	for verb, args := range m {
		s.Verb = verb
		s.Args = args
	}
	return nil
}

// CloneSteps returns a deep copy of steps, so a caller handed descriptor-owned
// steps cannot mutate the descriptor by writing through them.
func CloneSteps(steps []InjectStep) []InjectStep {
	out := make([]InjectStep, len(steps))
	for i, step := range steps {
		out[i] = InjectStep{Verb: step.Verb, Args: make(map[string]any, len(step.Args))}
		for key, value := range step.Args {
			out[i].Args[key] = value
		}
	}
	return out
}

// ArgString renders an inject-step argument as a string. A nil value is empty;
// anything not already a string is formatted with %v, which keeps YAML scalars
// (ints, bools) usable in templates without the caller type-switching.
func ArgString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
