package schema_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/schema"
)

func load(t *testing.T) schema.Vocabulary {
	t.Helper()
	v, err := schema.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return v
}

func TestLoad_CarriesEveryCanonicalEvent(t *testing.T) {
	v := load(t)
	// The full set the Go constants declared before this table existed, plus the two
	// compaction events and the three outbound ones.
	for _, name := range []string{
		"session_start", "user_prompt", "turn_stop", "turn_failed",
		"tool_pre", "tool_post", "tool_fail",
		"subagent_pre", "subagent_post", "notification",
		"message_delta", "session_end", "telemetry",
		"compact_pre", "compact_post",
		"permission", "elicitation",
		"prompt", "interrupt", "compact_start",
	} {
		if _, ok := v.Events[name]; !ok {
			t.Errorf("vocabulary is missing %q", name)
		}
	}
}

func TestLoad_EveryEventDeclaresADirection(t *testing.T) {
	v := load(t)
	for name, rule := range v.Events {
		switch rule.Direction {
		case "in", "out", "ask":
		default:
			t.Errorf("event %q has direction %q, want in|out|ask", name, rule.Direction)
		}
	}
}

// These three are the rules rules/hook_vocabulary.go enforced in Go. They must survive
// the move to data or descriptors silently lose their required fields.
func TestValidate_CarriesTheThreeRulesThatWereInGo(t *testing.T) {
	v := load(t)
	for _, tc := range []struct{ event, missing string }{
		{"session_start", "session_id"},
		{"turn_stop", "message"},
		{"message_delta", "message_id"},
	} {
		err := v.Validate("acme", map[string]map[string]string{tc.event: {}})
		if err == nil {
			t.Errorf("%s with no %s must be rejected", tc.event, tc.missing)
			continue
		}
		if !strings.Contains(err.Error(), tc.missing) {
			t.Errorf("%s: error must name %q, got: %v", tc.event, tc.missing, err)
		}
		if !strings.Contains(err.Error(), "acme") {
			t.Errorf("%s: error must name the provider, got: %v", tc.event, err)
		}
	}
}

func TestValidate_UnknownEvent_IsRejected(t *testing.T) {
	v := load(t)
	err := v.Validate("acme", map[string]map[string]string{
		"invent_a_new_event": {"session_id": "session_id"},
	})
	if err == nil {
		t.Fatal("the vocabulary is CLOSED; an unknown event must be rejected")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Fatalf("the error should say why, got: %v", err)
	}
}

func TestValidate_UnknownFieldWithinAKnownEvent_IsRejected(t *testing.T) {
	v := load(t)
	err := v.Validate("acme", map[string]map[string]string{
		"session_start": {"session_id": "session_id", "not_a_field": "x"},
	})
	if err == nil {
		t.Fatal("a field outside required+optional must be rejected, or a typo maps silently to nothing")
	}
}

// suggestion_label.* is a PREFIX FAMILY: its keys are the provider's own machine names
// for a broader grant, which Go must never enumerate.
func TestValidate_PrefixFamilyAcceptsAnyMemberButNotAnUnrelatedField(t *testing.T) {
	v := load(t)
	ok := map[string]map[string]string{"permission": {
		"prompt_id":                       "prompt_id",
		"suggestion_label.addRules":       "Add a permanent rule for this",
		"suggestion_label.somethingNewer": "A term this provider invented",
	}}
	if err := v.Validate("acme", ok); err != nil {
		t.Fatalf("a prefix family must accept any member: %v", err)
	}

	bad := map[string]map[string]string{"permission": {
		"prompt_id":           "prompt_id",
		"suggestion_labelXXX": "not under the family",
	}}
	if err := v.Validate("acme", bad); err == nil {
		t.Fatal("a near-miss on the family prefix must still be rejected")
	}
}

// Capability is key-presence: declaring no telemetry is legal, not an error.
func TestValidate_APartialProviderIsAccepted(t *testing.T) {
	v := load(t)
	if err := v.Validate("acme", map[string]map[string]string{
		"session_start": {"session_id": "session_id"},
		"turn_stop":     {"message": "msg"},
	}); err != nil {
		t.Fatalf("a partial provider must be accepted, got: %v", err)
	}
}

// Telemetry's canonical names are DOTTED (context.used_tokens). If the validator split
// on dots it would reject every real telemetry mapping.
func TestValidate_DottedCanonicalFieldsAreAccepted(t *testing.T) {
	v := load(t)
	if err := v.Validate("acme", map[string]map[string]string{
		"telemetry": {
			"context.used_tokens": "context_window.total_input_tokens",
			"cost.total_usd":      "cost.total_cost_usd",
		},
	}); err != nil {
		t.Fatalf("dotted canonical names must be accepted, got: %v", err)
	}
}

// The same descriptor must produce the same error every run, or the test that asserts
// on it is flaky.
func TestValidate_ErrorIsDeterministic(t *testing.T) {
	v := load(t)
	in := map[string]map[string]string{
		"session_start": {"a": "1", "b": "2", "c": "3", "d": "4"},
		"turn_stop":     {"z": "9", "y": "8"},
	}
	first := v.Validate("acme", in).Error()
	for range 20 {
		if got := v.Validate("acme", in).Error(); got != first {
			t.Fatalf("error is not deterministic:\n  %s\n  %s", first, got)
		}
	}
}

func TestRepliesFor_NamesTheDecisionsAnAskEventAccepts(t *testing.T) {
	v := load(t)
	got := v.Events["permission"].Replies
	want := map[string]bool{"allow": true, "deny": true, "answer": true}
	if len(got) != len(want) {
		t.Fatalf("permission replies = %v", got)
	}
	for _, r := range got {
		if !want[r] {
			t.Errorf("unexpected reply %q", r)
		}
	}
}
