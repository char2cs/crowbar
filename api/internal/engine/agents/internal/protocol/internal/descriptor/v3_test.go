package descriptor_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const minimalV3 = `
id: acme
display_name: Acme
protocol_version: { min: "1.0", max: "1.9" }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
  spawn:
    cmd: acme
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map: { message: turn.lastAgentMessage }
  tool_pre:
    in: item/started
    when: { item.type: commandExecution || fileChange }
    map: { tool_id: item.id }
  permission:
    ask: approval/request
    timeout_seconds: 270
    map: { prompt_id: "$rpc.id" }
    reply: { allow: '{"decision":"approved"}', deny: '{"decision":"denied"}' }
  compact_start:
    out: thread/compact/start
    send: { threadId: "{session_id}" }
`

func mustParse(t *testing.T, y string) *spec.Descriptor {
	t.Helper()
	d, err := descriptor.ParseV3([]byte(y))
	if err != nil {
		t.Fatalf("ParseV3: %v", err)
	}
	return d
}

func TestParseV3_LoadsEachDirection(t *testing.T) {
	d := mustParse(t, minimalV3)
	if got := d.Events["session_start"].In; got != "thread/started" {
		t.Errorf("session_start.in = %q", got)
	}
	if got := d.Events["compact_start"].Out; got != "thread/compact/start" {
		t.Errorf("compact_start.out = %q", got)
	}
	if got := d.Events["permission"].Ask; got != "approval/request" {
		t.Errorf("permission.ask = %q", got)
	}
	if got := d.Events["permission"].TimeoutSeconds; got != 270 {
		t.Errorf("permission.timeout_seconds = %d", got)
	}
	if got := d.Events["permission"].Reply["allow"]; got == "" {
		t.Error("permission.reply.allow is empty")
	}
	if got := d.Events["tool_pre"].When["item.type"]; got != "commandExecution || fileChange" {
		t.Errorf("tool_pre.when = %q", got)
	}
	if got := d.Events["compact_start"].Send["threadId"]; got != "{session_id}" {
		t.Errorf("compact_start.send = %q", got)
	}
}

func TestParseV3_RejectsAnEventOutsideTheVocabulary(t *testing.T) {
	bad := strings.Replace(minimalV3, "  session_start:", "  not_an_event:", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("the vocabulary is closed; an unknown event must be rejected at load")
	}
}

func TestParseV3_RejectsAMissingRequiredField(t *testing.T) {
	bad := strings.Replace(minimalV3, "map: { message: turn.lastAgentMessage }", "map: {}", 1)
	_, err := descriptor.ParseV3([]byte(bad))
	if err == nil {
		t.Fatal("turn_stop must map message")
	}
	if !strings.Contains(err.Error(), "message") {
		t.Fatalf("error must name the field, got: %v", err)
	}
}

func TestParseV3_RejectsAReplyTheEventDoesNotDeclare(t *testing.T) {
	bad := strings.Replace(minimalV3,
		`reply: { allow: '{"decision":"approved"}', deny: '{"decision":"denied"}' }`,
		`reply: { allow: '{"decision":"approved"}', shrug: '{}' }`, 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("a reply key outside the event's declared decisions must be rejected")
	}
}

func TestParseV3_RejectsADirectionMismatch(t *testing.T) {
	// compact_start is an `out` event; declaring it with `in:` is a descriptor bug.
	bad := strings.Replace(minimalV3, "    out: thread/compact/start", "    in: thread/compact/start", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("an out-event declared with in: must be rejected")
	}
}

func TestParseV3_RejectsAnEventDeclaringNoWireName(t *testing.T) {
	bad := strings.Replace(minimalV3, "    in: thread/started\n", "", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("an event with no in:/out:/ask: names nothing and must be rejected")
	}
}

// Per-event transport is what makes a MIXED provider possible with no new concept.
func TestParseV3_PerEventTransportOverridesTheRuntimeDefault(t *testing.T) {
	mixed := strings.Replace(minimalV3,
		"  compact_start:\n    out: thread/compact/start",
		"  compact_start:\n    transport: hooks\n    out: thread/compact/start", 1)
	d := mustParse(t, mixed)
	if got := d.TransportFor("compact_start"); got != "hooks" {
		t.Errorf("TransportFor(compact_start) = %q, want hooks", got)
	}
	if got := d.TransportFor("session_start"); got != "api" {
		t.Errorf("TransportFor(session_start) = %q, want the runtime default api", got)
	}
	if got := d.TransportFor("not_declared"); got != "api" {
		t.Errorf("TransportFor of an undeclared event = %q, want the runtime default", got)
	}
}

func TestCheckProtocolVersion(t *testing.T) {
	d := mustParse(t, minimalV3)
	for _, tc := range []struct {
		actual string
		wantOK bool
	}{
		{"1.0", true},
		{"1.5", true},
		{"1.9", true},
		{"2.4", false},
		{"0.9", false},
		{"", true}, // a provider that reports no version is not gated
	} {
		err := descriptor.CheckProtocolVersion(d, tc.actual)
		if tc.wantOK && err != nil {
			t.Errorf("version %q should be accepted: %v", tc.actual, err)
		}
		if !tc.wantOK && err == nil {
			t.Errorf("version %q should be refused", tc.actual)
		}
	}
}

// A dotted-numeric compare, not a string compare: "1.10" is NEWER than "1.9".
func TestCheckProtocolVersion_ComparesNumericallyNotLexically(t *testing.T) {
	y := strings.Replace(minimalV3, `min: "1.0", max: "1.9"`, `min: "1.2", max: "1.10"`, 1)
	d := mustParse(t, y)
	if err := descriptor.CheckProtocolVersion(d, "1.9"); err != nil {
		t.Fatalf("1.9 is inside [1.2, 1.10] numerically: %v", err)
	}
	if err := descriptor.CheckProtocolVersion(d, "1.11"); err == nil {
		t.Fatal("1.11 is outside [1.2, 1.10]")
	}
}

// A descriptor that declares no range accepts anything — providers without a readable
// version must not be blocked.
func TestCheckProtocolVersion_NoRangeAcceptsAnything(t *testing.T) {
	y := strings.Replace(minimalV3, `protocol_version: { min: "1.0", max: "1.9" }`, "", 1)
	d := mustParse(t, y)
	if err := descriptor.CheckProtocolVersion(d, "99.0"); err != nil {
		t.Fatalf("no declared range must accept any version: %v", err)
	}
}
