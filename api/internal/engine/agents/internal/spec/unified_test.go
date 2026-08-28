package spec_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func v3() *spec.Descriptor {
	no := false
	return &spec.Descriptor{
		ID: "acme",
		Runtime: spec.RuntimeSpec{
			Transport: "hooks",
			Hooks: spec.HooksWire{
				Format:               "json",
				RequirePayloadFields: []string{"transcript_path"},
			},
		},
		Events: map[string]spec.EventSpec{
			"session_start": {In: "SessionStart", Map: map[string]string{"session_id": "session_id"}},
			"permission": {
				Ask: "PermissionRequest", TimeoutSeconds: 270, AnswersInto: "answers",
				Reply: map[string]string{"allow": "{}"},
			},
			"observed_only": {Ask: "Watched", Answerable: &no, Map: map[string]string{}},
			"compact_start": {Out: "prompt", Send: map[string]string{"text": "/compact"}},
		},
	}
}

func TestEventFields_ReadsTheEventTable(t *testing.T) {
	d := v3()
	f, ok := d.EventFields("session_start")
	if !ok || f["session_id"] != "session_id" {
		t.Errorf("EventFields = (%v,%v)", f, ok)
	}
	if _, ok := d.EventFields("never_declared"); ok {
		t.Error("an undeclared event must report false — key-presence IS the capability check")
	}
}

// Outbound events are things Crowbar SENDS. Listing them as observations would make
// Capabilities claim the provider reports something it does not.
func TestDeclaredEvents_ExcludesOutboundAndIsSorted(t *testing.T) {
	got := v3().DeclaredEvents()
	want := []string{"observed_only", "permission", "session_start"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (sorted, no outbound)", got, want)
		}
	}
}

func TestAnswerFor_ReturnsTheDecisionChannel(t *testing.T) {
	a, ok := v3().AnswerFor("permission")
	if !ok || a.TimeoutSeconds != 270 || a.AnswersInto != "answers" || a.Responses["allow"] == "" {
		t.Fatalf("v3 AnswerFor = (%+v,%v)", a, ok)
	}
}

// answerable:false is the declared form of "visible but a decision reaches nobody",
// which is what a missing v2 answer block meant silently.
func TestAnswerFor_RefusesAnObservedOnlyEvent(t *testing.T) {
	if _, ok := v3().AnswerFor("observed_only"); ok {
		t.Fatal("answerable:false must not report an answer channel")
	}
	if _, ok := v3().AnswerFor("session_start"); ok {
		t.Fatal("an inbound event is not answerable")
	}
	if _, ok := v3().AnswerFor("absent"); ok {
		t.Fatal("an undeclared event is not answerable")
	}
}

func TestWireName_ReturnsTheProvidersOwnName(t *testing.T) {
	d := v3()
	for canonical, want := range map[string]string{
		"session_start": "SessionStart",
		"permission":    "PermissionRequest",
		"compact_start": "prompt",
		"absent":        "",
	} {
		if got := d.WireName(canonical); got != want {
			t.Errorf("WireName(%q) = %q, want %q", canonical, got, want)
		}
	}
}

func TestHookFormatAndRequiredFields(t *testing.T) {
	d := v3()
	if got := d.HookFormat(); got != "json" {
		t.Errorf("HookFormat = %q", got)
	}
	got := d.RequiredPayloadFields()
	if len(got) != 1 || got[0] != "transcript_path" {
		t.Errorf("RequiredPayloadFields = %v", got)
	}
}

func TestTransportFor_FallsBackToTheRuntimeDefault(t *testing.T) {
	d := v3()
	if got := d.TransportFor("session_start"); got != "hooks" {
		t.Errorf("TransportFor = %q, want the runtime default", got)
	}
	e := d.Events["session_start"]
	e.Transport = "api"
	d.Events["session_start"] = e
	if got := d.TransportFor("session_start"); got != "api" {
		t.Errorf("a per-event transport must win, got %q", got)
	}
}
