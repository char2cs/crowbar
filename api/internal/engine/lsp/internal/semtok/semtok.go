package semtok

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var TokenTypes = []string{
	"namespace", "type", "class", "enum", "interface", "struct", "typeParameter",
	"parameter", "variable", "property", "enumMember", "event", "function",
	"method", "macro", "keyword", "modifier", "comment", "string", "number",
	"regexp", "operator", "decorator", "label", "tag", "attribute",
}

var TokenModifiers = []string{
	"readonly", "declaration", "definition", "static", "deprecated", "abstract",
	"async", "modification", "documentation", "defaultLibrary",
}

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

var modifierAliases = map[string]string{
	"constant": "readonly",
	"library":  "defaultLibrary",
}

var ErrMisalignedDelta = errors.New("semtok: delta edits are not token-aligned")

type Support struct {
	full   bool
	delta  bool
	rng    bool
	types  []uint32
	modMap []uint32
}

func (s Support) Full() bool { return s.full }

func (s Support) Delta() bool { return s.delta }

func (s Support) Range() bool { return s.rng }

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

type Tokens struct {
	ResultID string   `json:"resultId,omitempty"`
	Data     []uint32 `json:"data"`
}

type Edit struct {
	Start       uint32   `json:"start"`
	DeleteCount uint32   `json:"deleteCount"`
	Data        []uint32 `json:"data,omitempty"`
}

type Delta struct {
	ResultID string `json:"resultId,omitempty"`
	Edits    []Edit `json:"edits"`
}

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

func (s Support) remapTokens(
	data []uint32,
) {
	s.remapFrom(data, 0)
}

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
