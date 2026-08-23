package mapping_test

import (
	"encoding/json"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
)

// mapping replaces payload. For any path WITHOUT alternation the two must agree
// exactly, or porting the callers in stage 3 changes behaviour silently.
//
// Alternation is deliberately excluded: v2 spelled it with a comma and v3 spells it
// `||`, so the two disagree there by design (see TestString_CommaIsNotAnOperator).
func TestMapping_AgreesWithPayloadOnPlainPaths(t *testing.T) {
	raw := `{
		"session_id": "s-1",
		"tool_use_id": "t-9",
		"duration_ms": 1234,
		"ok": false,
		"tool_input": {"command": "ls -la", "file_path": "", "nested": {"deep": "yes"}},
		"tool_response": {"stdout": "hi"},
		"questions": [{"header": "Pick", "multiSelect": true}],
		"when": "2026-08-23T10:00:00Z"
	}`
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		"session_id", "tool_use_id", "duration_ms", "ok",
		"tool_input", "tool_input.command", "tool_input.file_path",
		"tool_input.nested.deep", "tool_response", "tool_response.stdout",
		"questions", "when",
		"", "absent", "absent.deeper", "tool_input.command.notanobject",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			if a, b := mapping.String(doc, p), payload.String(doc, p); a != b {
				t.Errorf("String(%q): mapping=%q payload=%q", p, a, b)
			}
			ai, aok := mapping.Int(doc, p)
			bi, bok := payload.Int(doc, p)
			if ai != bi || aok != bok {
				t.Errorf("Int(%q): mapping=(%d,%v) payload=(%d,%v)", p, ai, aok, bi, bok)
			}
			ab, aok2 := mapping.Bool(doc, p)
			bb, bok2 := payload.Bool(doc, p)
			if ab != bb || aok2 != bok2 {
				t.Errorf("Bool(%q): mapping=(%v,%v) payload=(%v,%v)", p, ab, aok2, bb, bok2)
			}
			if a, b := string(mapping.JSON(doc, p)), string(payload.JSON(doc, p)); a != b {
				t.Errorf("JSON(%q): mapping=%q payload=%q", p, a, b)
			}
			if a, b := mapping.Count(doc, p), payload.Count(doc, p); a != b {
				t.Errorf("Count(%q): mapping=%d payload=%d", p, a, b)
			}
			as, aok3 := mapping.Scalar(doc, p)
			bs, bok3 := payload.Scalar(doc, p)
			if as != bs || aok3 != bok3 {
				t.Errorf("Scalar(%q): mapping=(%q,%v) payload=(%q,%v)", p, as, aok3, bs, bok3)
			}
			if a, b := len(mapping.Objects(doc, p)), len(payload.Objects(doc, p)); a != b {
				t.Errorf("Objects(%q): mapping=%d payload=%d", p, a, b)
			}
			at, aok4 := mapping.Time(doc, p)
			bt, bok4 := payload.Time(doc, p)
			if !at.Equal(bt) || aok4 != bok4 {
				t.Errorf("Time(%q): mapping=(%v,%v) payload=(%v,%v)", p, at, aok4, bt, bok4)
			}
		})
	}
}
