package spec

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// firstPresentOperator/anyOfOperator are the explicit, ONLY spellings of alternation:
// first_present: for map:, any_of: for when:. The `||` glyph they replaced is not
// merely deprecated — it is a hard parse error (AlternationGlyphError), everywhere an
// expression can appear (docs/plans/2026-09-22-descriptor-channel-split.md 5.0).
const (
	firstPresentOperator = "first_present"
	anyOfOperator        = "any_of"
)

// AlternationGlyphError is returned the moment a descriptor spells alternation with
// `||` instead of first_present:/any_of:/a list — anywhere: a map: value, a when:
// value, an in:/ask:/out: wire ref. Event is filled in by EventTable.UnmarshalYAML,
// which is the first point in the decode that knows which event this field belongs
// to; Field and Value are known immediately, at the point the glyph is found.
type AlternationGlyphError struct {
	Event string
	Field string
	Value string
}

func (e *AlternationGlyphError) Error() string {
	event := e.Event
	if event == "" {
		event = "<unknown>"
	}
	return fmt.Sprintf(
		"event %q field %q: %q contains the \"||\" glyph — alternation is spelled "+
			"first_present:/any_of:/a list, never joined with ||",
		event, e.Field, e.Value,
	)
}

// EventTable is Descriptor.Events' concrete type. Its own UnmarshalYAML decodes
// event by event, purely so a bad field's error can name which EVENT it came from —
// the map key, invisible to a field's own UnmarshalYAML (FieldMap/WhenMap/WireRef),
// which only ever sees the value node.
type EventTable map[string]EventSpec

func (t *EventTable) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("spec: events: want a mapping, got %v", node.Kind)
	}
	out := make(EventTable, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := node.Content[i].Value
		var ev EventSpec
		if err := node.Content[i+1].Decode(&ev); err != nil {
			var glyph *AlternationGlyphError
			if errors.As(err, &glyph) {
				glyph.Event = name
				return glyph
			}
			return fmt.Errorf("event %q: %w", name, err)
		}
		out[name] = ev
	}
	*t = out
	return nil
}

// FieldMap is an event's map: block: canonical field name to its alternative source
// paths, in declared order — first path PRESENT in the payload wins. A plain scalar
// value is a one-path list; `{first_present: [a, b, c]}` spells more than one.
type FieldMap map[string][]string

func (m *FieldMap) UnmarshalYAML(node *yaml.Node) error {
	decoded, err := decodeExprMap(node, firstPresentOperator)
	if err != nil {
		return fmt.Errorf("spec: map: %w", err)
	}
	*m = decoded
	return nil
}

// WhenMap is an event's when: block: discriminator path to the value(s) it must
// equal. A plain scalar value is a one-value list; `{any_of: [a, b]}` spells more
// than one.
type WhenMap map[string][]string

func (m *WhenMap) UnmarshalYAML(node *yaml.Node) error {
	decoded, err := decodeExprMap(node, anyOfOperator)
	if err != nil {
		return fmt.Errorf("spec: when: %w", err)
	}
	*m = decoded
	return nil
}

func decodeExprMap(node *yaml.Node, operator string) (map[string][]string, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("want a mapping, got %v", node.Kind)
	}
	out := make(map[string][]string, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		paths, err := decodeExpr(node.Content[i+1], operator, key)
		if err != nil {
			var glyph *AlternationGlyphError
			if errors.As(err, &glyph) {
				return nil, glyph
			}
			return nil, fmt.Errorf("%q: %w", key, err)
		}
		out[key] = paths
	}
	return out, nil
}

// decodeExpr accepts a plain path/value scalar or the one operator mapping this
// block admits. Anything else — a sequence, a document, the wrong operator name —
// is a descriptor bug and fails the load rather than silently mapping to nothing. A
// scalar spelling `||` is not "anything else" — that would silently map to a path
// that never resolves — so it gets its own named error.
func decodeExpr(node *yaml.Node, operator, field string) ([]string, error) {
	switch node.Kind { //nolint:exhaustive // default rejects every other node kind with the same error.
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return nil, err
		}
		if strings.Contains(s, "||") {
			return nil, &AlternationGlyphError{Field: field, Value: s}
		}
		return []string{s}, nil
	case yaml.MappingNode:
		return decodeOperator(node, operator, field)
	default:
		return nil, fmt.Errorf("want a string or {%s: [...]}, got %v", operator, node.Kind)
	}
}

func decodeOperator(node *yaml.Node, operator, field string) ([]string, error) {
	if len(node.Content) != 2 {
		return nil, fmt.Errorf("an operator mapping needs exactly one key, got %d", len(node.Content)/2)
	}
	if key := node.Content[0].Value; key != operator {
		return nil, fmt.Errorf("unknown operator %q, want %q", key, operator)
	}
	var paths []string
	if err := node.Content[1].Decode(&paths); err != nil {
		return nil, fmt.Errorf("%s: value must be a list of paths: %w", operator, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s: must name at least one path", operator)
	}
	for _, p := range paths {
		if strings.Contains(p, "||") {
			return nil, &AlternationGlyphError{Field: field, Value: p}
		}
	}
	return paths, nil
}
