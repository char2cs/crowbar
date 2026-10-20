package spec

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
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

// FieldMap is an event's map: block: canonical field name to its alternative branches, the
// first one PRESENT in the payload winning. A branch is a path or a transform
// (`{from: p, pattern: re}`, `{join: [a, b]}`); `{first_present: [...]}` lists several.
type FieldMap map[string]mapping.Expr

func (m *FieldMap) UnmarshalYAML(node *yaml.Node) error {
	decoded, err := decodeTable(node, decodeBranches)
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
	decoded, err := decodeTable(node, decodeValues)
	if err != nil {
		return fmt.Errorf("spec: when: %w", err)
	}
	*m = decoded
	return nil
}

// decodeTable decodes a mapping of field name to expression; a bad field's error names it.
func decodeTable[V any](node *yaml.Node, decode func(*yaml.Node, string) (V, error)) (map[string]V, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("want a mapping, got %v", node.Kind)
	}
	out := make(map[string]V, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		v, err := decode(node.Content[i+1], key)
		if err != nil {
			var glyph *AlternationGlyphError
			if errors.As(err, &glyph) {
				return nil, glyph
			}
			return nil, fmt.Errorf("%q: %w", key, err)
		}
		out[key] = v
	}
	return out, nil
}

// decodeValues accepts a plain value scalar or the one any_of mapping. Anything else is
// a descriptor bug and fails the load; a scalar spelling `||` gets its own named error
// because it would otherwise silently map to a value that never matches.
func decodeValues(node *yaml.Node, field string) ([]string, error) {
	switch node.Kind { //nolint:exhaustive // default rejects every other node kind with the same error.
	case yaml.ScalarNode:
		s, err := decodeScalar(node, field)
		return []string{s}, err
	case yaml.MappingNode:
		list, err := operatorList(node, anyOfOperator)
		if err != nil {
			return nil, err
		}
		var values []string
		if err := list.Decode(&values); err != nil {
			return nil, fmt.Errorf("%s: value must be a list of paths: %w", anyOfOperator, err)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("%s: must name at least one path", anyOfOperator)
		}
		return values, rejectGlyph(values, field)
	default:
		return nil, fmt.Errorf("want a string or {%s: [...]}, got %v", anyOfOperator, node.Kind)
	}
}

// decodeBranches accepts a plain path, one transform, or the first_present list of either.
func decodeBranches(node *yaml.Node, field string) (mapping.Expr, error) {
	switch node.Kind { //nolint:exhaustive // default rejects every other node kind with the same error.
	case yaml.ScalarNode:
		branch, err := decodeBranch(node, field)
		return mapping.Expr{branch}, err
	case yaml.MappingNode:
		if branch, ok, err := decodeTransform(node, field); ok {
			return mapping.Expr{branch}, err
		}
		list, err := operatorList(node, firstPresentOperator)
		if err != nil {
			return nil, err
		}
		return decodeBranchList(list, field)
	default:
		return nil, fmt.Errorf("want a string or {%s: [...]}, got %v", firstPresentOperator, node.Kind)
	}
}

func decodeBranchList(list *yaml.Node, field string) (mapping.Expr, error) {
	if list.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: value must be a list of paths or transforms", firstPresentOperator)
	}
	if len(list.Content) == 0 {
		return nil, fmt.Errorf("%s: must name at least one path", firstPresentOperator)
	}
	out := make(mapping.Expr, len(list.Content))
	for i, item := range list.Content {
		branch, err := decodeBranch(item, field)
		if err != nil {
			return nil, err
		}
		out[i] = branch
	}
	return out, nil
}

// decodeBranch reads one alternative: a path scalar or a transform mapping.
func decodeBranch(node *yaml.Node, field string) (mapping.Branch, error) {
	if node.Kind == yaml.ScalarNode {
		path, err := decodeScalar(node, field)
		return mapping.Path(path), err
	}
	branch, ok, err := decodeTransform(node, field)
	if !ok && err == nil {
		err = errors.New("an alternative is a path, {from, pattern} or {join: [...]}")
	}
	return branch, err
}

func decodeScalar(node *yaml.Node, field string) (string, error) {
	var s string
	if err := node.Decode(&s); err != nil {
		return "", err
	}
	if strings.Contains(s, "||") {
		return "", &AlternationGlyphError{Field: field, Value: s}
	}
	return s, nil
}

// operatorList checks node is a mapping of exactly the one operator and returns its list.
func operatorList(node *yaml.Node, operator string) (*yaml.Node, error) {
	if len(node.Content) != 2 {
		return nil, fmt.Errorf("an operator mapping needs exactly one key, got %d", len(node.Content)/2)
	}
	if key := node.Content[0].Value; key != operator {
		return nil, fmt.Errorf("unknown operator %q, want %q", key, operator)
	}
	return node.Content[1], nil
}

func rejectGlyph(paths []string, field string) error {
	for _, p := range paths {
		if strings.Contains(p, "||") {
			return &AlternationGlyphError{Field: field, Value: p}
		}
	}
	return nil
}

// decodeTransform reads the two spellings that compute a value instead of naming a
// path; ok is false for any other mapping.
func decodeTransform(node *yaml.Node, field string) (branch mapping.Branch, ok bool, err error) {
	keys := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keys[node.Content[i].Value] = node.Content[i+1]
	}
	switch {
	case len(keys) == 2 && keys["from"] != nil && keys["pattern"] != nil:
		branch, err = decodePattern(keys["from"], keys["pattern"], field)
		return branch, true, err
	case len(keys) == 1 && keys["join"] != nil:
		branch, err = decodeJoin(keys["join"], field)
		return branch, true, err
	}
	return mapping.Branch{}, false, nil
}

func decodePattern(from, pattern *yaml.Node, field string) (mapping.Branch, error) {
	var path, re string
	if err := from.Decode(&path); err != nil {
		return mapping.Branch{}, fmt.Errorf("from: %w", err)
	}
	if err := pattern.Decode(&re); err != nil {
		return mapping.Branch{}, fmt.Errorf("pattern: %w", err)
	}
	if err := rejectGlyph([]string{path}, field); err != nil {
		return mapping.Branch{}, err
	}
	return mapping.Pattern(path, re)
}

func decodeJoin(node *yaml.Node, field string) (mapping.Branch, error) {
	var paths []string
	if err := node.Decode(&paths); err != nil {
		return mapping.Branch{}, fmt.Errorf("join: value must be a list of paths: %w", err)
	}
	if len(paths) == 0 {
		return mapping.Branch{}, errors.New("join: must name at least one path")
	}
	return mapping.Join(paths...), rejectGlyph(paths, field)
}
