package ledger

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

// previewed names, per kind, the body fields a closed row paints without: a
// preview cuts them, the full row stays in the ledger.
var previewed = map[Kind][]string{
	KindToolCall:  {"diff", "output"},
	KindTurnEnded: {"diff"},
	KindReasoning: {"text"},
	KindSubagent:  {"result"},
}

// PreviewKinds are the kinds of row Preview may cut.
func PreviewKinds() []Kind { return slices.Sorted(maps.Keys(previewed)) }

// Preview is r with each previewable body field longer than limit bytes cut to
// at most limit, at a line when it has one, and named in Trimmed. A row with
// nothing to cut is r itself; r's body is never modified.
func Preview(r Row, limit int) Row {
	fields := previewed[r.Kind]
	if len(fields) == 0 || len(r.Body) <= limit {
		return r
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return r
	}
	var trimmed []string
	for _, name := range fields {
		var text string
		if json.Unmarshal(body[name], &text) != nil || len(text) <= limit {
			continue
		}
		cut, err := json.Marshal(cutText(text, limit))
		if err != nil {
			continue
		}
		body[name] = cut
		trimmed = append(trimmed, name)
	}
	if len(trimmed) == 0 {
		return r
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return r
	}
	r.Body, r.Trimmed = raw, trimmed
	return r
}

// cutText is text's prefix of at most limit bytes, ending at a line when it has
// one and never inside a rune.
func cutText(text string, limit int) string {
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		return text[:i+1]
	}
	return text
}
