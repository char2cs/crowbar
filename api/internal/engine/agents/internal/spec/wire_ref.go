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

// WireRef is the provider's own name for one canonical event — or, where a provider
// splits a single conversational fact across several wire methods, every name that
// fact answers to, in declared order. A descriptor spells more than one name as a
// YAML list (`ask: [a, b]`); Names/Name/Has/Empty read either shape identically.
//
// Multiple names exist because the canonical vocabulary is CLOSED: a provider cannot
// answer a namespaced wire method by inventing a Crowbar event per namespace. Codex
// asks for tool approval under item/commandExecution/requestApproval for a command
// and item/fileChange/requestApproval for a patch — one canonical `permission` fact,
// two wire names. A descriptor naming only one of them leaves the other matching
// nothing at all: no prompt is raised, no reply is ever sent, and the CLI blocks on
// an answer that cannot arrive until its approval budget expires (measured live
// against codex-cli 0.149.1 — a turn sat waitingOnApproval indefinitely).
//
// Every event that has exactly one wire name still declares it as the plain string
// it always was; WireRef carries it as a one-element list.
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
