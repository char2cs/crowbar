package spec

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// wireRefField is the Field an AlternationGlyphError reports for a bad in:/ask:/out:
// — WireRef's own UnmarshalYAML is shared by all three and does not know which one it
// is decoding, so it names the mechanism rather than guessing a tag.
const wireRefField = "in/ask/out"

// WireRef is the provider's wire name(s) for one canonical event: a plain string, or a
// YAML list when one fact spans several wire methods (the vocabulary is closed, so
// there is no event per namespace). An unlisted name would leave the CLI blocked.
type WireRef []string

// UnmarshalYAML accepts a plain scalar (one wire name) or a YAML list (several wire
// names for one canonical event).
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

// Names is every wire name the event answers to, in declared order.
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

// Name is the first declared wire name: what a caller with room for exactly one uses,
// since an outbound call names a single method and a label wants a single word.
func (w WireRef) Name() string {
	names := w.Names()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// Has reports whether wireMethod is one of the names this event answers to.
func (w WireRef) Has(wireMethod string) bool {
	for _, name := range w.Names() {
		if name == wireMethod {
			return true
		}
	}
	return false
}

// Empty reports whether the event names nothing in this direction.
func (w WireRef) Empty() bool {
	return w.Name() == ""
}
