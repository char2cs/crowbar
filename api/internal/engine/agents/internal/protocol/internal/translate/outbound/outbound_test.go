package outbound_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func desc() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "acme",
		Events: map[string]spec.EventSpec{
			"compact_start": {Out: "prompt", Send: map[string]string{"text": "/compact"}},
			"interrupt":     {Out: "turn/interrupt", Send: map[string]string{"threadId": "{session_id}"}},
			"prompt": {
				Out:  "turn/start",
				Send: map[string]string{"threadId": "{session_id}", "input": "{text}"},
			},
			"session_start": {In: "SessionStart", Map: map[string]string{"session_id": "session_id"}},
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

// Declared lists what Crowbar can SEND, which is what a capability flag is built from.
func TestDeclared_ListsOnlyOutboundEventsSorted(t *testing.T) {
	got := outbound.Declared(desc())
	want := []string{"compact_start", "interrupt", "prompt"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
