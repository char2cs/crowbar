package spec

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const wireRefField = "in/ask/out"

type WireRef []string

func (w *WireRef) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind { //nolint:exhaustive // default rejects every other node kind with the same error.
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return fmt.Errorf("spec: wire ref: %w", err)
		}
		if strings.Contains(s, "||") {
			return &AlternationGlyphError{Field: wireRefField, Value: s}
		}
		*w = WireRef{s}
		return nil
	case yaml.SequenceNode:
		var names []string
		if err := node.Decode(&names); err != nil {
			return fmt.Errorf("spec: wire ref: %w", err)
		}
		if len(names) == 0 {
			return fmt.Errorf("spec: wire ref: a list must name at least one wire method")
		}
		for _, n := range names {
			if strings.Contains(n, "||") {
				return &AlternationGlyphError{Field: wireRefField, Value: n}
			}
		}
		*w = WireRef(names)
		return nil
	default:
		return fmt.Errorf("spec: wire ref: want a string or a list of strings, got %v", node.Kind)
	}
}

func (w WireRef) Names() []string {
	var out []string
	for _, part := range w {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func (w WireRef) Name() string {
	names := w.Names()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func (w WireRef) Has(wireMethod string) bool {
	for _, name := range w.Names() {
		if name == wireMethod {
			return true
		}
	}
	return false
}

func (w WireRef) Empty() bool {
	return w.Name() == ""
}
