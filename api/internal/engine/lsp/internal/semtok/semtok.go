// Package semtok rewrites each language server's semantic-token legend into
// Crowbar's one canonical legend: Monaco fixes a provider's legend once, and
// the rewrite only touches the type/modifier slot of each token, so full,
// range and delta results all translate element-wise. A type with no
// canonical counterpart gets an out-of-legend index, which Monaco leaves
// unstyled (the syntax color stays).
package semtok

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// TokenTypes is the canonical legend's token types: the LSP 3.18 standard set
// plus the markup types (tag, attribute) the editor's grammar tier emits. The
// web legend (features/editor/monaco/semantic-tokens-legend.ts) mirrors it
// index for index; both sides pin the list in a test.
var TokenTypes = []string{
	"namespace", "type", "class", "enum", "interface", "struct", "typeParameter",
	"parameter", "variable", "property", "enumMember", "event", "function",
	"method", "macro", "keyword", "modifier", "comment", "string", "number",
	"regexp", "operator", "decorator", "label", "tag", "attribute",
}

// TokenModifiers is the canonical legend's modifiers. readonly comes first on
// purpose: Monaco styles a token by matching "type.mod1.mod2" (modifiers in
// legend order) against theme rules by longest dotted prefix, so a
// "variable.readonly" rule only matches when readonly is the first modifier.
var TokenModifiers = []string{
	"readonly", "declaration", "definition", "static", "deprecated", "abstract",
	"async", "modification", "documentation", "defaultLibrary",
}

// typeAliases maps well-known non-standard server token types to their nearest
// canonical type (rust-analyzer, clangd, jdtls). Anything absent from both
// TokenTypes and this table is left unstyled.
var typeAliases = map[string]string{
	"builtinType":      "type",
	"typeAlias":        "type",
	"concept":          "type",
	"union":            "struct",
	"trait":            "interface",
	"record":           "class",
	"recordComponent":  "property",
	"annotation":       "decorator",
	"annotationMember": "method",
	"builtinAttribute": "attribute",
	"derive":           "decorator",
	"lifetime":         "typeParameter",
	"constParameter":   "typeParameter",
	"selfKeyword":      "keyword",
	"selfTypeKeyword":  "keyword",
	"boolean":          "keyword",
	"toolModule":       "namespace",
}

// modifierAliases maps well-known non-standard modifiers to canonical ones.
var modifierAliases = map[string]string{
	"constant": "readonly",
	"library":  "defaultLibrary",
}

// ErrMisalignedDelta reports a delta whose edits move surviving tokens off
// their 5-int boundaries, so its slots cannot be rewritten element-wise; the
// caller asks the server for a full result instead.
var ErrMisalignedDelta = errors.New("semtok: delta edits are not token-aligned")

// Support is what one language server offers for semantic tokens, with the
// index tables that rewrite its legend into the canonical one. The zero value
// supports nothing.
type Support struct {
	full   bool
	delta  bool
	rng    bool
	types  []uint32
	modMap []uint32
}

// Full reports whether the server answers textDocument/semanticTokens/full.
func (s Support) Full() bool { return s.full }

// Delta reports whether the server answers textDocument/semanticTokens/full/delta.
func (s Support) Delta() bool { return s.delta }

// Range reports whether the server answers textDocument/semanticTokens/range.
func (s Support) Range() bool { return s.rng }

// FromCapabilities reads semanticTokensProvider from a server's initialize
// result capabilities. Anything malformed or absent yields the zero Support.
func FromCapabilities(
	capabilities json.RawMessage,
) Support {
	var caps struct {
		Provider *struct {
			Legend struct {
				TokenTypes     []string `json:"tokenTypes"`
				TokenModifiers []string `json:"tokenModifiers"`
			} `json:"legend"`
			Full  json.RawMessage `json:"full"`
			Range json.RawMessage `json:"range"`
		} `json:"semanticTokensProvider"`
	}
	if err := json.Unmarshal(capabilities, &caps); err != nil || caps.Provider == nil {
		return Support{}
	}
	p := caps.Provider
	full, delta := parseFull(p.Full)
	return Support{
		full:   full,
		delta:  delta,
		rng:    enabled(p.Range),
		types:  typeTable(p.Legend.TokenTypes),
		modMap: modifierTable(p.Legend.TokenModifiers),
	}
}

// enabled reads an LSP `boolean | {}` option: true or any object means on.
func enabled(
	raw json.RawMessage,
) bool {
	if len(raw) == 0 {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b
	}
	var obj map[string]json.RawMessage
	return json.Unmarshal(raw, &obj) == nil && obj != nil
}

// parseFull reads the `full: boolean | { delta?: boolean }` option.
func parseFull(
	raw json.RawMessage,
) (full, delta bool) {
	if !enabled(raw) {
		return false, false
	}
	var obj struct {
		Delta bool `json:"delta"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return true, obj.Delta
	}
	return true, false
}

func typeTable(
	serverTypes []string,
) []uint32 {
	canonical := make(map[string]uint32, len(TokenTypes))
	for i, t := range TokenTypes {
		canonical[t] = uint32(i) // #nosec G115 -- a fixed legend of a few dozen names
	}
	unstyled := uint32(len(TokenTypes)) // #nosec G115 -- as above
	table := make([]uint32, len(serverTypes))
	for i, t := range serverTypes {
		if alias, ok := typeAliases[t]; ok {
			t = alias
		}
		idx, ok := canonical[t]
		if !ok {
			idx = unstyled
		}
		table[i] = idx
	}
	return table
}

func modifierTable(
	serverModifiers []string,
) []uint32 {
	canonical := make(map[string]uint32, len(TokenModifiers))
	for i, m := range TokenModifiers {
		canonical[m] = 1 << uint(i) // #nosec G115 -- a fixed legend of ten names
	}
	table := make([]uint32, len(serverModifiers))
	for i, m := range serverModifiers {
		if alias, ok := modifierAliases[m]; ok {
			m = alias
		}
		table[i] = canonical[m]
	}
	return table
}

// Tokens is a full or range result: 5 ints per token (deltaLine,
// deltaStartChar, length, tokenType, tokenModifiers).
type Tokens struct {
	ResultID string   `json:"resultId,omitempty"`
	Data     []uint32 `json:"data"`
}

// Edit is one edit of a delta result, addressed into the previous result's
// data array.
type Edit struct {
	Start       uint32   `json:"start"`
	DeleteCount uint32   `json:"deleteCount"`
	Data        []uint32 `json:"data,omitempty"`
}

// Delta is a full/delta result: edits against the previous result.
type Delta struct {
	ResultID string `json:"resultId,omitempty"`
	Edits    []Edit `json:"edits"`
}

// Remap decodes a full, range or delta result, rewrites it into the canonical
// legend and re-encodes it. A null result stays nil. A delta that cannot be
// rewritten element-wise fails with ErrMisalignedDelta.
func (s Support) Remap(
	raw json.RawMessage,
) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var r struct {
		ResultID string   `json:"resultId"`
		Data     []uint32 `json:"data"`
		Edits    *[]Edit  `json:"edits"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("semtok: decode: %w", err)
	}
	if r.Edits != nil {
		if err := s.remapDelta(*r.Edits); err != nil {
			return nil, err
		}
		return encode(Delta{ResultID: r.ResultID, Edits: *r.Edits})
	}
	if r.Data == nil {
		r.Data = []uint32{}
	}
	s.remapTokens(r.Data)
	return encode(Tokens{ResultID: r.ResultID, Data: r.Data})
}

func encode(
	result any,
) (json.RawMessage, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("semtok: encode: %w", err)
	}
	return encoded, nil
}

// remapTokens rewrites data (a full or range result's array) into the
// canonical legend in place.
func (s Support) remapTokens(
	data []uint32,
) {
	s.remapFrom(data, 0)
}

// remapDelta rewrites every edit's inserted data into the canonical legend in
// place. An element's slot (type, modifiers, or a position) is its index in the
// NEW array modulo 5, so each edit's offset is shifted by the net growth of the
// edits before it. The previous result was already canonical, so the elements
// the edits keep need no rewrite — unless an earlier edit shifted them off
// their slot, which ErrMisalignedDelta reports.
func (s Support) remapDelta(
	edits []Edit,
) error {
	order := make([]int, len(edits))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return edits[order[a]].Start < edits[order[b]].Start })
	shift := 0
	for _, i := range order {
		if shift%5 != 0 {
			return ErrMisalignedDelta
		}
		e := edits[i]
		s.remapFrom(e.Data, int(e.Start)+shift)
		shift += len(e.Data) - int(e.DeleteCount)
	}
	if shift%5 != 0 {
		return ErrMisalignedDelta
	}
	return nil
}

// remapFrom rewrites data whose first element sits at index offset of a token
// array.
func (s Support) remapFrom(
	data []uint32,
	offset int,
) {
	for k := range data {
		switch (offset + k) % 5 {
		case 3:
			data[k] = s.mapType(data[k])
		case 4:
			data[k] = s.mapModifiers(data[k])
		}
	}
}

func (s Support) mapType(
	t uint32,
) uint32 {
	if int(t) < len(s.types) {
		return s.types[t]
	}
	return uint32(len(TokenTypes)) // #nosec G115 -- a fixed legend of a few dozen names
}

func (s Support) mapModifiers(
	set uint32,
) uint32 {
	var out uint32
	for bit := 0; set != 0 && bit < len(s.modMap); bit++ {
		if set&1 != 0 {
			out |= s.modMap[bit]
		}
		set >>= 1
	}
	return out
}
