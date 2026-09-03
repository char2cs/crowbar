package mapping_test

import (
	"encoding/json"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
)

func doc() map[string]any {
	return map[string]any{
		"session_id": "s-1",
		"tool_input": map[string]any{
			"command":   "ls -la",
			"file_path": "",
		},
		"turn":  map[string]any{"lastAgentMessage": "done"},
		"item":  map[string]any{"type": "commandExecution", "id": "i-1"},
		"usage": map[string]any{"inputTokens": float64(1200)},
		"questions": []any{
			map[string]any{"header": "Pick", "multiSelect": true},
		},
		"done": true,
	}
}

func TestString_WalksADottedPath(t *testing.T) {
	if got := mapping.String(doc(), "turn.lastAgentMessage"); got != "done" {
		t.Fatalf("got %q, want done", got)
	}
}

func TestString_AlternationTakesTheFirstNonEmpty(t *testing.T) {
	got := mapping.String(doc(), "tool_input.file_path || tool_input.command")
	if got != "ls -la" {
		t.Fatalf("got %q, want the first NON-EMPTY branch", got)
	}
}

func TestString_AlternationSkipsMissingAsWellAsEmpty(t *testing.T) {
	if got := mapping.String(doc(), "nope.nothing || tool_input.command"); got != "ls -la" {
		t.Fatalf("got %q, want ls -la", got)
	}
}

func TestString_MissingPathIsEmptyNotAPanic(t *testing.T) {
	if got := mapping.String(doc(), "a.b.c.d"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// The v2 grammar overloaded comma as alternation, which made a comma-bearing key
// unaddressable. v3 uses || so this works.
func TestString_CommaIsNotAnOperator(t *testing.T) {
	d := map[string]any{"a,b": "kept"}
	if got := mapping.String(d, "a,b"); got != "kept" {
		t.Fatalf("got %q, want kept", got)
	}
}

// A dotted CANONICAL name on the left is not a path; but a dotted key in the payload
// must still resolve whole-key-first, or telemetry's own shapes break.
func TestString_WholeKeyWinsOverSegmentWalk(t *testing.T) {
	d := map[string]any{
		"a.b": "whole",
		"a":   map[string]any{"b": "walked"},
	}
	if got := mapping.String(d, "a.b"); got != "whole" {
		t.Fatalf("got %q, want the whole-key match", got)
	}
}

func TestInt_ReadsANumericLeaf(t *testing.T) {
	got, ok := mapping.Int(doc(), "usage.inputTokens")
	if !ok || got != 1200 {
		t.Fatalf("got (%d,%v), want (1200,true)", got, ok)
	}
}

func TestBool_ReadsABooleanLeaf(t *testing.T) {
	got, ok := mapping.Bool(doc(), "done")
	if !ok || !got {
		t.Fatalf("got (%v,%v), want (true,true)", got, ok)
	}
}

func TestObjects_ReadsAnArrayOfObjects(t *testing.T) {
	got := mapping.Objects(doc(), "questions")
	if len(got) != 1 || got[0]["header"] != "Pick" {
		t.Fatalf("got %+v", got)
	}
}

func TestJSON_MarshalsANestedObject(t *testing.T) {
	if got := mapping.JSON(doc(), "item"); len(got) == 0 {
		t.Fatal("want the item object as JSON bytes")
	}
}

func TestMatch_SelectsOnAVariantField(t *testing.T) {
	if !mapping.Match(doc(), map[string]string{"item.type": "commandExecution"}) {
		t.Fatal("want a match on item.type")
	}
	if mapping.Match(doc(), map[string]string{"item.type": "fileChange"}) {
		t.Fatal("must not match a different variant")
	}
}

func TestMatch_AlternationInTheWhenValue(t *testing.T) {
	when := map[string]string{"item.type": "fileChange || commandExecution"}
	if !mapping.Match(doc(), when) {
		t.Fatal("a when: value may alternate; commandExecution is in the set")
	}
}

// An event with no when: applies unconditionally — otherwise every existing mapping
// would stop firing the day when: was introduced.
func TestMatch_EmptyWhenMatchesEverything(t *testing.T) {
	if !mapping.Match(doc(), nil) {
		t.Fatal("no when: means unconditional")
	}
	if !mapping.Match(doc(), map[string]string{}) {
		t.Fatal("an empty when: means unconditional")
	}
}

func TestMatch_AllClausesMustHold(t *testing.T) {
	when := map[string]string{"item.type": "commandExecution", "session_id": "other"}
	if mapping.Match(doc(), when) {
		t.Fatal("when: is a conjunction; one failing clause fails the match")
	}
}

// A missing path in a when: clause must not match, or a variant selector silently
// applies to every payload that lacks the discriminator.
func TestMatch_MissingDiscriminatorDoesNotMatch(t *testing.T) {
	if mapping.Match(doc(), map[string]string{"absent.field": "anything"}) {
		t.Fatal("a missing discriminator must not match")
	}
}

// --- array selection -------------------------------------------------------
//
// Real provider payloads put the interesting value inside a list. Codex's final
// agent message is turn.items[type=agentMessage].text, and its user message text is
// item.content[type=text].text. Without selection those are unmappable and the
// provider needs Go.

func arrayDoc() map[string]any {
	return map[string]any{
		"turn": map[string]any{
			"items": []any{
				map[string]any{"type": "reasoning", "text": "thinking"},
				map[string]any{"type": "agentMessage", "text": "OK", "phase": "final_answer"},
			},
		},
		"item": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "Reply with exactly: OK"},
			},
		},
		"empty": map[string]any{"items": []any{}},
	}
}

func TestString_SelectsFromAnArrayByField(t *testing.T) {
	if got := mapping.String(arrayDoc(), "turn.items[type=agentMessage].text"); got != "OK" {
		t.Fatalf("got %q, want OK", got)
	}
}

func TestString_ArraySelectionTakesTheFirstMatch(t *testing.T) {
	if got := mapping.String(arrayDoc(), "turn.items[type=reasoning].text"); got != "thinking" {
		t.Fatalf("got %q, want thinking", got)
	}
}

func TestString_ArraySelectionWithNoMatchIsEmpty(t *testing.T) {
	if got := mapping.String(arrayDoc(), "turn.items[type=nothingLikeThis].text"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestString_ArraySelectionOnAnEmptyListIsEmpty(t *testing.T) {
	if got := mapping.String(arrayDoc(), "empty.items[type=x].y"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestString_ArraySelectionComposesWithAlternation(t *testing.T) {
	expr := "turn.items[type=missing].text || item.content[type=text].text"
	if got := mapping.String(arrayDoc(), expr); got != "Reply with exactly: OK" {
		t.Fatalf("got %q", got)
	}
}

// A bracket in a plain key must not be mistaken for a selector.
func TestString_ABracketedKeyIsStillAddressable(t *testing.T) {
	d := map[string]any{"weird[key]": "kept"}
	if got := mapping.String(d, "weird[key]"); got != "kept" {
		t.Fatalf("got %q, want kept (whole-key match wins)", got)
	}
}

// --- coverage of the numeric and object accessors --------------------------

func TestFloat_AcceptsEveryNumericShapeAPayloadCanCarry(t *testing.T) {
	d := map[string]any{
		"f64": float64(1.5),
		"f32": float32(2.5),
		"i":   int(3),
		"i64": int64(4),
		"num": json.Number("5.5"),
		"bad": json.Number("not-a-number"),
		"str": "6",
	}
	for _, tc := range []struct {
		path   string
		want   float64
		wantOK bool
	}{
		{"f64", 1.5, true},
		{"f32", 2.5, true},
		{"i", 3, true},
		{"i64", 4, true},
		{"num", 5.5, true},
		{"bad", 0, false},
		{"str", 0, false},
		{"absent", 0, false},
	} {
		got, ok := mapping.Float(d, tc.path)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("Float(%q) = (%v,%v), want (%v,%v)", tc.path, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestScalar_RendersEveryScalarShape(t *testing.T) {
	d := map[string]any{
		"s": "text", "b": true, "f64": float64(1.5), "f32": float32(2.5),
		"i": int(3), "i64": int64(4), "num": json.Number("7"),
		"nil": nil, "obj": map[string]any{"a": 1},
	}
	for _, tc := range []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"s", "text", true},
		{"b", "true", true},
		{"f64", "1.5", true},
		{"f32", "2.5", true},
		{"i", "3", true},
		{"i64", "4", true},
		{"num", "7", true},
		{"nil", "", false},
		{"obj", "", false},
		{"absent", "", false},
	} {
		got, ok := mapping.Scalar(d, tc.path)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("Scalar(%q) = (%q,%v), want (%q,%v)", tc.path, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestObject_ReturnsANestedObjectOrNil(t *testing.T) {
	d := map[string]any{"obj": map[string]any{"a": "1"}, "notobj": "text"}
	if got := mapping.Object(d, "obj"); got == nil || got["a"] != "1" {
		t.Fatalf("Object(obj) = %+v", got)
	}
	if got := mapping.Object(d, "notobj"); got != nil {
		t.Fatalf("Object of a non-object should be nil, got %+v", got)
	}
	if got := mapping.Object(d, "absent"); got != nil {
		t.Fatalf("Object of a missing path should be nil, got %+v", got)
	}
}

// --- coverage of the selector grammar's malformed shapes --------------------

// A bracketed segment that is not a valid `field=value` selector (no `=`, or
// an empty field name before it) must fall back to a PLAIN key rather than
// being treated as a selector — parseSelector's own contract.
func TestString_ASelectorSegmentWithNoEqualsIsAPlainKey(t *testing.T) {
	d := map[string]any{"arr": []any{map[string]any{"a": "1"}}}
	if got := mapping.String(d, "arr[novalue]"); got != "" {
		t.Fatalf("got %q, want empty: a bracket with no '=' is not a selector", got)
	}
}

func TestString_ASelectorSegmentWithAnEmptyFieldIsAPlainKey(t *testing.T) {
	d := map[string]any{"arr": []any{map[string]any{"a": "1"}}}
	if got := mapping.String(d, "arr[=novalue]"); got != "" {
		t.Fatalf("got %q, want empty: a selector needs a non-empty field name", got)
	}
}

// selectFrom must fail closed, not panic, when the path segment BEFORE the
// selector does not resolve to a list at all.
func TestString_ArraySelectionOnANonArrayIsEmpty(t *testing.T) {
	d := map[string]any{"obj": "not a list"}
	if got := mapping.String(d, "obj[type=foo]"); got != "" {
		t.Fatalf("got %q, want empty: a selector over a non-array must not resolve", got)
	}
}

// An element of the list that is not itself an object must be skipped, not
// mistaken for a match or a panic — real payloads mix shapes inside one array.
func TestString_ArraySelectionSkipsNonObjectElements(t *testing.T) {
	d := map[string]any{
		"items": []any{"not an object", map[string]any{"type": "foo", "val": "kept"}},
	}
	if got := mapping.String(d, "items[type=foo].val"); got != "kept" {
		t.Fatalf("got %q, want kept: the non-object element must be skipped, not matched", got)
	}
}

// --- coverage of the JSON and Objects accessors' failure branches ----------

// A present-but-EMPTY string leaf renders as no bytes at all, distinct from a
// leaf holding the literal two-byte string `""`.
func TestJSON_AnEmptyStringLeafIsNilNotEmptyQuotes(t *testing.T) {
	d := map[string]any{"s": ""}
	if got := mapping.JSON(d, "s"); got != nil {
		t.Fatalf("got %q, want nil for an empty string leaf", got)
	}
}

// A leaf holding a value json.Marshal cannot encode (a channel, here — a
// shape resolve() never produces from real JSON, but the accessor still must
// not panic on it) must fail closed to nil rather than propagating the
// encoding error nowhere the caller could see it.
func TestJSON_AnUnmarshalableLeafIsNil(t *testing.T) {
	d := map[string]any{"bad": make(chan int)}
	if got := mapping.JSON(d, "bad"); got != nil {
		t.Fatalf("got %q, want nil for a value json.Marshal cannot encode", got)
	}
}

// Objects on a leaf that resolves but is not an array must yield nil, the
// same "fail closed" contract as a missing path.
func TestObjects_ANonArrayLeafIsNilNotAPanic(t *testing.T) {
	d := map[string]any{"notarray": "text"}
	if got := mapping.Objects(d, "notarray"); got != nil {
		t.Fatalf("got %+v, want nil for a non-array leaf", got)
	}
}

// A false bool is an ANSWER, not an absence: alternation must not skip past it.
func TestIsEmpty_OnlyNilAndEmptyStringCountAsEmpty(t *testing.T) {
	d := map[string]any{"f": false, "zero": float64(0), "blank": "", "nil": nil}
	if got, ok := mapping.Bool(d, "f || nil"); !ok || got {
		t.Fatalf("Bool(f||nil) = (%v,%v), want (false,true): false is an answer", got, ok)
	}
	if got, ok := mapping.Float(d, "zero"); !ok || got != 0 {
		t.Fatalf("Float(zero) = (%v,%v), want (0,true): zero is an answer", got, ok)
	}
	if got := mapping.String(d, "blank || nil"); got != "" {
		t.Fatalf("all-empty alternation should be empty, got %q", got)
	}
}
