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

func v2() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "legacy",
		Hooks: spec.HookSpec{
			Format:               "json",
			RequirePayloadFields: []string{"transcript_path"},
			Events: map[string]map[string]string{
				"session_start": {"session_id": "session_id"},
			},
		},
		Answer: spec.AnswerSpec{
			"permission": {TimeoutSeconds: 90, Responses: map[string]string{"allow": "{}"}},
		},
	}
}

func TestIsV3_DistinguishesTheShapes(t *testing.T) {
	if !v3().IsV3() {
		t.Error("a descriptor with events is v3")
	}
	if v2().IsV3() {
		t.Error("a descriptor with no events is v2")
	}
}

func TestEventFields_ReadsEitherShape(t *testing.T) {
	for name, d := range map[string]*spec.Descriptor{"v3": v3(), "v2": v2()} {
		f, ok := d.EventFields("session_start")
		if !ok || f["session_id"] != "session_id" {
			t.Errorf("%s: EventFields = (%v,%v)", name, f, ok)
		}
		if _, ok := d.EventFields("never_declared"); ok {
			t.Errorf("%s: an undeclared event must report false — key-presence IS the capability check", name)
		}
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
	if legacy := v2().DeclaredEvents(); len(legacy) != 1 || legacy[0] != "session_start" {
		t.Fatalf("v2 declared events = %v", legacy)
	}
}

func TestAnswerFor_BothShapes(t *testing.T) {
	a, ok := v3().AnswerFor("permission")
	if !ok || a.TimeoutSeconds != 270 || a.AnswersInto != "answers" || a.Responses["allow"] == "" {
		t.Fatalf("v3 AnswerFor = (%+v,%v)", a, ok)
	}
	if l, ok := v2().AnswerFor("permission"); !ok || l.TimeoutSeconds != 90 {
		t.Fatalf("v2 AnswerFor = (%+v,%v)", l, ok)
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
	// A v2 descriptor keeps its wire names in config injection, not the event table.
	if got := v2().WireName("session_start"); got != "" {
		t.Errorf("v2 WireName = %q, want empty", got)
	}
}

func TestHookFormatAndRequiredFields_ReadEitherShape(t *testing.T) {
	for name, d := range map[string]*spec.Descriptor{"v3": v3(), "v2": v2()} {
		if got := d.HookFormat(); got != "json" {
			t.Errorf("%s: HookFormat = %q", name, got)
		}
		got := d.RequiredPayloadFields()
		if len(got) != 1 || got[0] != "transcript_path" {
			t.Errorf("%s: RequiredPayloadFields = %v", name, got)
		}
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
