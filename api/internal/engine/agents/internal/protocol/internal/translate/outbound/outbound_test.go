package outbound_test

import (
	"encoding/json"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func desc() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "acme",
		Events: map[string]spec.EventSpec{
			"compact_start": {Out: spec.WireRef{"prompt"}, Send: map[string]any{"text": "/compact"}},
			"interrupt":     {Out: spec.WireRef{"turn/interrupt"}, Send: map[string]any{"threadId": "{session_id}"}},
			"prompt": {
				Out:  spec.WireRef{"turn/start"},
				Send: map[string]any{"threadId": "{session_id}", "input": "{text}"},
			},
			"session_start": {In: spec.WireRef{"SessionStart"}, Map: spec.FieldMap{"session_id": mapping.Paths("session_id")}},
		},
	}
}

func TestResolve_ReturnsTheWireEventAndTheFilledPayload(t *testing.T) {
	wire, send, ok := outbound.Resolve(desc(), "interrupt", map[string]string{"session_id": "s-1"})
	if !ok {
		t.Fatal("interrupt is declared and must resolve")
	}
	if wire != "turn/interrupt" {
		t.Fatalf("wire = %q", wire)
	}
	if send["threadId"] != "s-1" {
		t.Fatalf("threadId = %q, want the substituted session id", send["threadId"])
	}
}

// Capability is key-presence: an undeclared outbound event must report false rather
// than silently producing an empty call.
func TestResolve_AnUndeclaredEventReportsFalse(t *testing.T) {
	if _, _, ok := outbound.Resolve(desc(), "never_declared", nil); ok {
		t.Fatal("an undeclared event must not resolve")
	}
}

// An INBOUND event is not something Crowbar can send.
func TestResolve_AnInboundEventIsNotSendable(t *testing.T) {
	if _, _, ok := outbound.Resolve(desc(), "session_start", nil); ok {
		t.Fatal("an inbound event must not resolve as outbound")
	}
}

// A literal with no placeholder passes through untouched — this is how claude's
// compact_start carries the bare slash command.
func TestResolve_ALiteralPassesThroughUnchanged(t *testing.T) {
	wire, send, ok := outbound.Resolve(desc(), "compact_start", nil)
	if !ok || wire != "prompt" {
		t.Fatalf("got (%q,%v)", wire, ok)
	}
	if send["text"] != "/compact" {
		t.Fatalf("text = %q, want /compact", send["text"])
	}
}

// An unsupplied placeholder must resolve to empty, not leave the brace in the payload —
// a provider receiving a literal "{session_id}" is a bug that reaches the wire.
func TestResolve_AnUnsuppliedPlaceholderBecomesEmpty(t *testing.T) {
	_, send, ok := outbound.Resolve(desc(), "interrupt", nil)
	if !ok {
		t.Fatal("resolve")
	}
	if send["threadId"] != "" {
		t.Fatalf("threadId = %q, want empty — never a literal placeholder", send["threadId"])
	}
}

func TestResolve_SubstitutesEveryOccurrence(t *testing.T) {
	_, send, _ := outbound.Resolve(desc(), "prompt", map[string]string{
		"session_id": "s-9", "text": "hello",
	})
	if send["threadId"] != "s-9" || send["input"] != "hello" {
		t.Fatalf("got %+v", send)
	}
}

// A leaf that is exactly one {name} keeps a captured number or bool as that JSON type;
// anything mixed, a string capture, or a caller-supplied value stays a string.
func TestResolveTyped_AnExactPlaceholderKeepsTheCapturedJSONType(t *testing.T) {
	d := &spec.Descriptor{ID: "acme", Events: map[string]spec.EventSpec{
		"interrupt": {Out: spec.WireRef{"cancel"}, Send: map[string]any{
			"generation": "{gen}", "flag": "{on}", "id": "{session_id}",
			"label": "g{gen}", "retries": 3, "fenced": true,
		}},
	}}
	values := map[string]string{"gen": "7", "on": "true", "session_id": "12345"}

	wire, send, ok := outbound.ResolveTyped(d, "interrupt", values, map[string]bool{"gen": true, "on": true})

	if !ok || wire != "cancel" {
		t.Fatalf("got (%q,%v)", wire, ok)
	}
	got, err := json.Marshal(send)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"fenced":true,"flag":true,"generation":7,"id":"12345","label":"g7","retries":3}`
	if string(got) != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}

// The string-only view (hooks transports) renders typed leaves as their text.
func TestResolve_ATypedLiteralRendersAsText(t *testing.T) {
	d := &spec.Descriptor{ID: "acme", Events: map[string]spec.EventSpec{
		"interrupt": {Out: spec.WireRef{"cancel"}, Send: map[string]any{"retries": 3}},
	}}
	_, send, _ := outbound.Resolve(d, "interrupt", nil)
	if send["retries"] != "3" {
		t.Fatalf("retries = %q", send["retries"])
	}
}
