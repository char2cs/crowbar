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

func p(paths ...string) mapping.Expr { return mapping.Paths(paths...) }

func TestString_WalksADottedPath(t *testing.T) {
	if got := mapping.String(doc(), p("turn.lastAgentMessage")); got != "done" {
		t.Fatalf("got %q, want done", got)
	}
}

func TestString_AlternationTakesTheFirstNonEmpty(t *testing.T) {
	got := mapping.String(doc(), p("tool_input.file_path", "tool_input.command"))
	if got != "ls -la" {
		t.Fatalf("got %q, want the first NON-EMPTY branch", got)
	}
}

func TestString_AlternationSkipsMissingAsWellAsEmpty(t *testing.T) {
	if got := mapping.String(doc(), p("nope.nothing", "tool_input.command")); got != "ls -la" {
		t.Fatalf("got %q, want ls -la", got)
	}
}

func TestString_MissingPathIsEmptyNotAPanic(t *testing.T) {
	if got := mapping.String(doc(), p("a.b.c.d")); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestString_NoBranchesIsEmpty(t *testing.T) {
	if got := mapping.String(doc(), nil); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// A comma is part of the path, never an alternation operator.
func TestString_CommaIsNotAnOperator(t *testing.T) {
	d := map[string]any{"a,b": "kept"}
	if got := mapping.String(d, p("a,b")); got != "kept" {
		t.Fatalf("got %q, want kept", got)
	}
}

// A dotted payload key resolves whole-key-first.
func TestString_WholeKeyWinsOverSegmentWalk(t *testing.T) {
	d := map[string]any{
		"a.b": "whole",
		"a":   map[string]any{"b": "walked"},
	}
	if got := mapping.String(d, p("a.b")); got != "whole" {
		t.Fatalf("got %q, want the whole-key match", got)
	}
}

func TestInt_ReadsANumericLeaf(t *testing.T) {
	got, ok := mapping.Int(doc(), p("usage.inputTokens"))
	if !ok || got != 1200 {
		t.Fatalf("got (%d,%v), want (1200,true)", got, ok)
	}
}

func TestBool_ReadsABooleanLeaf(t *testing.T) {
	got, ok := mapping.Bool(doc(), p("done"))
	if !ok || !got {
		t.Fatalf("got (%v,%v), want (true,true)", got, ok)
	}
}

func TestObjects_ReadsAnArrayOfObjects(t *testing.T) {
	got := mapping.Objects(doc(), p("questions"))
	if len(got) != 1 || got[0]["header"] != "Pick" {
		t.Fatalf("got %+v", got)
	}
}

func TestJSON_MarshalsANestedObject(t *testing.T) {
	if got := mapping.JSON(doc(), p("item")); len(got) == 0 {
		t.Fatal("want the item object as JSON bytes")
	}
}

func TestMatch_SelectsOnAVariantField(t *testing.T) {
	if !mapping.Match(doc(), map[string][]string{"item.type": {"commandExecution"}}) {
		t.Fatal("want a match on item.type")
	}
	if mapping.Match(doc(), map[string][]string{"item.type": {"fileChange"}}) {
		t.Fatal("must not match a different variant")
	}
}

func TestMatch_MatchesAnyValueInTheSet(t *testing.T) {
	when := map[string][]string{"item.type": {"fileChange", "commandExecution"}}
	if !mapping.Match(doc(), when) {
		t.Fatal("a when: value may name a set; commandExecution is in it")
	}
}

// An event with no when: applies unconditionally.
func TestMatch_EmptyWhenMatchesEverything(t *testing.T) {
	if !mapping.Match(doc(), nil) {
		t.Fatal("no when: means unconditional")
	}
	if !mapping.Match(doc(), map[string][]string{}) {
		t.Fatal("an empty when: means unconditional")
	}
}

func TestMatch_AllClausesMustHold(t *testing.T) {
	when := map[string][]string{"item.type": {"commandExecution"}, "session_id": {"other"}}
	if mapping.Match(doc(), when) {
		t.Fatal("when: is a conjunction; one failing clause fails the match")
	}
}

// A missing discriminator must not match, or a selector applies to every payload.
func TestMatch_MissingDiscriminatorDoesNotMatch(t *testing.T) {
	if mapping.Match(doc(), map[string][]string{"absent.field": {"anything"}}) {
		t.Fatal("a missing discriminator must not match")
	}
}

// --- array selection ---

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
	for path, want := range map[string]string{
		"turn.items[type=agentMessage].text":    "OK",
		"turn.items[type=reasoning].text":       "thinking",
		"turn.items[type=nothingLikeThis].text": "",
		"empty.items[type=x].y":                 "",
	} {
		if got := mapping.String(arrayDoc(), p(path)); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}

func TestString_ArraySelectionComposesWithAlternation(t *testing.T) {
	got := mapping.String(arrayDoc(), p("turn.items[type=missing].text", "item.content[type=text].text"))
	if got != "Reply with exactly: OK" {
		t.Fatalf("got %q", got)
	}
}

// A bracket in a plain key must not be mistaken for a selector.
func TestString_ABracketedKeyIsStillAddressable(t *testing.T) {
	d := map[string]any{"weird[key]": "kept"}
	if got := mapping.String(d, p("weird[key]")); got != "kept" {
		t.Fatalf("got %q, want kept (whole-key match wins)", got)
	}
}

// --- numeric and object accessors ---

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
		got, ok := mapping.Float(d, p(tc.path))
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
		got, ok := mapping.Scalar(d, p(tc.path))
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("Scalar(%q) = (%q,%v), want (%q,%v)", tc.path, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestObject_ReturnsANestedObjectOrNil(t *testing.T) {
	d := map[string]any{"obj": map[string]any{"a": "1"}, "notobj": "text"}
	if got := mapping.Object(d, p("obj")); got == nil || got["a"] != "1" {
		t.Fatalf("Object(obj) = %+v", got)
	}
	if got := mapping.Object(d, p("notobj")); got != nil {
		t.Fatalf("Object of a non-object should be nil, got %+v", got)
	}
	if got := mapping.Object(d, p("absent")); got != nil {
		t.Fatalf("Object of a missing path should be nil, got %+v", got)
	}
}

// --- malformed selector shapes ---

// A dynamic-key selector over an array fails closed to empty.
func TestString_ASelectorSegmentWithNoEqualsOverAnArrayIsEmpty(t *testing.T) {
	d := map[string]any{"arr": []any{map[string]any{"a": "1"}}}
	if got := mapping.String(d, p("arr[novalue]")); got != "" {
		t.Fatalf("got %q, want empty: a dynamic-key selector over an array must not resolve", got)
	}
}

func TestString_ASelectorSegmentWithAnEmptyFieldIsAPlainKey(t *testing.T) {
	d := map[string]any{"arr": []any{map[string]any{"a": "1"}}}
	if got := mapping.String(d, p("arr[=novalue]")); got != "" {
		t.Fatalf("got %q, want empty: a selector needs a non-empty field name", got)
	}
}

// A selector over a non-list fails closed.
func TestString_ArraySelectionOnANonArrayIsEmpty(t *testing.T) {
	d := map[string]any{"obj": "not a list"}
	if got := mapping.String(d, p("obj[type=foo]")); got != "" {
		t.Fatalf("got %q, want empty: a selector over a non-array must not resolve", got)
	}
}

// A non-object list element is skipped, not matched.
func TestString_ArraySelectionSkipsNonObjectElements(t *testing.T) {
	d := map[string]any{
		"items": []any{"not an object", map[string]any{"type": "foo", "val": "kept"}},
	}
	if got := mapping.String(d, p("items[type=foo].val")); got != "kept" {
		t.Fatalf("got %q, want kept: the non-object element must be skipped, not matched", got)
	}
}

// --- JSON and Objects failure branches ---

// An empty string leaf renders as no bytes, not as quotes.
func TestJSON_AnEmptyStringLeafIsNilNotEmptyQuotes(t *testing.T) {
	d := map[string]any{"s": ""}
	if got := mapping.JSON(d, p("s")); got != nil {
		t.Fatalf("got %q, want nil for an empty string leaf", got)
	}
}

// A value json.Marshal cannot encode fails closed to nil.
func TestJSON_AnUnmarshalableLeafIsNil(t *testing.T) {
	d := map[string]any{"bad": make(chan int)}
	if got := mapping.JSON(d, p("bad")); got != nil {
		t.Fatalf("got %q, want nil for a value json.Marshal cannot encode", got)
	}
}

// Objects on a non-array leaf is nil, like a missing path.
func TestObjects_ANonArrayLeafIsNilNotAPanic(t *testing.T) {
	d := map[string]any{"notarray": "text"}
	if got := mapping.Objects(d, p("notarray")); got != nil {
		t.Fatalf("got %+v, want nil for a non-array leaf", got)
	}
}

// A false bool is an ANSWER, not an absence: alternation must not skip past it.
func TestIsEmpty_OnlyNilAndEmptyStringCountAsEmpty(t *testing.T) {
	d := map[string]any{"f": false, "zero": float64(0), "blank": "", "nil": nil}
	if got, ok := mapping.Bool(d, p("f", "nil")); !ok || got {
		t.Fatalf("Bool(f,nil) = (%v,%v), want (false,true): false is an answer", got, ok)
	}
	if got, ok := mapping.Float(d, p("zero")); !ok || got != 0 {
		t.Fatalf("Float(zero) = (%v,%v), want (0,true): zero is an answer", got, ok)
	}
	if got := mapping.String(d, p("blank", "nil")); got != "" {
		t.Fatalf("all-empty alternation should be empty, got %q", got)
	}
}

// A list element addressable only by position: `kind` is an object, so no
// field=value selector can match it.
func changesDoc() map[string]any {
	var d map[string]any
	_ = json.Unmarshal([]byte(`{"item":{"changes":[
	  {"path":"/w/main.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@"},
	  {"path":"/w/util.go","kind":{"type":"add"}}
	]}}`), &d)
	return d
}

func TestString_SelectsFromAnArrayByIndex(t *testing.T) {
	if got := mapping.String(changesDoc(), p("item.changes[0].path")); got != "/w/main.go" {
		t.Fatalf("got %q, want /w/main.go", got)
	}
	if got := mapping.String(changesDoc(), p("item.changes[1].path")); got != "/w/util.go" {
		t.Fatalf("got %q, want /w/util.go", got)
	}
}

func TestString_IndexSelectionComposesWithAlternation(t *testing.T) {
	got := mapping.String(changesDoc(), p("item.command", "item.changes[0].path"))
	if got != "/w/main.go" {
		t.Fatalf("got %q, want the fallback branch to resolve", got)
	}
}

func TestString_IndexOutOfRangeIsEmptyNotAPanic(t *testing.T) {
	if got := mapping.String(changesDoc(), p("item.changes[9].path")); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// A nested selector must still reach a scalar under an object-valued key.
func TestString_SelectsANestedFieldOfAnIndexedElement(t *testing.T) {
	if got := mapping.String(changesDoc(), p("item.changes[0].kind.type")); got != "update" {
		t.Fatalf("got %q, want update", got)
	}
}

func TestPresent_SeparatesNullAndOrdinaryValuesFromAnAbsentKey(t *testing.T) {
	if !mapping.Present(map[string]any{"transcript_path": nil}, p("transcript_path")) {
		t.Fatal("a JSON null is a present key, not an absent one")
	}
	if mapping.Present(map[string]any{}, p("transcript_path")) {
		t.Fatal("a key the payload never carries must not read as present")
	}
	if !mapping.Present(doc(), p("session_id")) {
		t.Fatal("a non-empty value must read as present")
	}
}

// --- dynamic-key (map) selection: the key is named by a sibling field ---

func collabDoc() map[string]any {
	return map[string]any{
		"item": map[string]any{
			"type":              "collabAgentToolCall",
			"receiverThreadIds": []any{"thread-child-1"},
			"agentsStates": map[string]any{
				"thread-child-1": map[string]any{"status": "completed", "message": "done"},
				"thread-other":   map[string]any{"status": "failed", "message": "boom"},
			},
		},
	}
}

func TestString_SelectsAMapValueByADynamicKeyFromASiblingField(t *testing.T) {
	got := mapping.String(collabDoc(), p("item.agentsStates[receiverThreadIds[0]].status"))
	if got != "completed" {
		t.Fatalf("got %q, want completed", got)
	}
}

func TestString_DynamicKeySelectionReachesANestedField(t *testing.T) {
	got := mapping.String(collabDoc(), p("item.agentsStates[receiverThreadIds[0]].message"))
	if got != "done" {
		t.Fatalf("got %q, want done", got)
	}
}

func TestString_DynamicKeySelectionThatCannotResolveIsEmpty(t *testing.T) {
	for name, item := range map[string]map[string]any{
		"no entry under the key": {"childId": "gone", "states": map[string]any{"t1": map[string]any{"status": "ok"}}},
		"a non-object target":    {"childId": "x", "states": []any{"not a map"}},
		"an unresolved key path": {"states": map[string]any{"x": "y"}},
	} {
		d := map[string]any{"item": item}
		if got := mapping.String(d, p("item.states[childId].status")); got != "" {
			t.Errorf("%s: got %q, want empty", name, got)
		}
	}
}

func TestString_DynamicKeySelectorComposesWithAlternation(t *testing.T) {
	got := mapping.String(collabDoc(), p(
		"item.agentsStates[missing].status", "item.agentsStates[receiverThreadIds[0]].message"))
	if got != "done" {
		t.Fatalf("got %q, want the fallback branch to resolve", got)
	}
}
