package mapping_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/mapping"
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
