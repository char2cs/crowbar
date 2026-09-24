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
    when: { item.type: { any_of: [commandExecution, fileChange] } }
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
	if got := d.Events["session_start"].In.Name(); got != "thread/started" {
		t.Errorf("session_start.in = %q", got)
	}
	if got := d.Events["compact_start"].Out.Name(); got != "thread/compact/start" {
		t.Errorf("compact_start.out = %q", got)
	}
	if got := d.Events["permission"].Ask.Name(); got != "approval/request" {
		t.Errorf("permission.ask = %q", got)
	}
	if got := d.Events["permission"].TimeoutSeconds; got != 270 {
		t.Errorf("permission.timeout_seconds = %d", got)
	}
	if got := d.Events["permission"].Reply["allow"]; got == "" {
		t.Error("permission.reply.allow is empty")
	}
	if got := d.Events["tool_pre"].When["item.type"]; len(got) != 2 || got[0] != "commandExecution" || got[1] != "fileChange" {
		t.Errorf("tool_pre.when = %v", got)
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

// channelSplitTooPre is the design spec's own tool_pre example (docs/plans/
// 2026-09-22-descriptor-channel-split.md, 2.1), REPLACING minimalV3's own
// legacy-form tool_pre — proving a channel-scoped event coexists with the
// OTHER legacy flat-form events in the same descriptor (session_start,
// turn_stop, permission, compact_start), which is exactly what P2 promises
// ("legacy events keep working unchanged").
const channelSplitTooPre = `  tool_pre:
    required: [session_id, tool_id, tool_name]
    api:
      in: item/started
      when: { item.type: { any_of: [commandExecution, fileChange] } }
      map: { session_id: threadId, tool_id: item.id, tool_name: item.type }
      fixtures: [item-started.commandExecution.json]
    hooks:
      in: PreToolUse
      map: { session_id: session_id, tool_id: tool_use_id, tool_name: tool_name }
      fixtures: [pre-tool-use.bash.json]
`

const legacyToolPre = `  tool_pre:
    in: item/started
    when: { item.type: { any_of: [commandExecution, fileChange] } }
    map: { tool_id: item.id }
`

func withChannelSplitEvent(base string) string {
	return strings.Replace(base, legacyToolPre, channelSplitTooPre, 1)
}

func TestParseV3_ChannelBlockEventParsesAlongsideLegacyEvents(t *testing.T) {
	d := mustParse(t, withChannelSplitEvent(minimalV3))

	ev := d.Events["tool_pre"]
	if ev.API == nil || ev.Hooks == nil {
		t.Fatalf("tool_pre must carry both channel blocks: api=%v hooks=%v", ev.API, ev.Hooks)
	}
	if got := ev.API.In.Name(); got != "item/started" {
		t.Errorf("tool_pre.api.in = %q", got)
	}
	if got := ev.Hooks.In.Name(); got != "PreToolUse" {
		t.Errorf("tool_pre.hooks.in = %q", got)
	}
	if got := ev.Required; len(got) != 3 {
		t.Errorf("tool_pre.required = %v", got)
	}

	// Legacy events in the SAME descriptor are unaffected.
	if got := d.Events["session_start"].In.Name(); got != "thread/started" {
		t.Errorf("session_start.in = %q (legacy events must still parse unchanged)", got)
	}
}

// vocab.Validate must see the UNION of both channel blocks' mapped fields, or
// a bad field name in either block would silently escape validation.
func TestParseV3_RejectsAnUnknownFieldInAChannelBlock(t *testing.T) {
	bad := strings.Replace(withChannelSplitEvent(minimalV3),
		"map: { session_id: session_id, tool_id: tool_use_id, tool_name: tool_name }",
		"map: { session_id: session_id, tool_id: tool_use_id, tool_name: tool_name, no_such_field: x }", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("an unknown field in a channel block's map: must be rejected, same as a legacy event's")
	}
}

func TestParseV3_RejectsAnEventMixingFlatAndChannelForms(t *testing.T) {
	bad := strings.Replace(withChannelSplitEvent(minimalV3),
		"  tool_pre:\n    required:",
		"  tool_pre:\n    in: item/started\n    required:", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("an event declaring both a flat in: and channel blocks is ambiguous and must be rejected")
	}
}

func TestParseV3_RejectsAnEventMixingFlatMapAndChannelForms(t *testing.T) {
	bad := strings.Replace(withChannelSplitEvent(minimalV3),
		"  tool_pre:\n    required: [session_id, tool_id, tool_name]",
		"  tool_pre:\n    required: [session_id, tool_id, tool_name]\n    map: { tool_id: x }", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("an event declaring both a flat map: and channel blocks is ambiguous and must be rejected")
	}
}

// A channel-scoped event whose blocks name no wire method at all is exactly
// as broken as a legacy event with no in:/out:/ask: — checkEvent's "names
// nothing" rule must reach through channel blocks too.
func TestParseV3_RejectsAChannelBlockEventDeclaringNoWireName(t *testing.T) {
	bad := strings.Replace(withChannelSplitEvent(minimalV3),
		"    api:\n      in: item/started\n", "    api:\n", 1)
	bad = strings.Replace(bad, "    hooks:\n      in: PreToolUse\n", "    hooks:\n", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("channel blocks naming no in: on either channel must be rejected")
	}
}

// legacyPermission is minimalV3's own flat permission event, replaced below by
// its channel-split counterpart the same way channelSplitTooPre replaces
// legacyToolPre.
const legacyPermission = `  permission:
    ask: approval/request
    timeout_seconds: 270
    map: { prompt_id: "$rpc.id" }
    reply: { allow: '{"decision":"approved"}', deny: '{"decision":"denied"}' }
`

// channelSplitPermission mirrors codex.yaml's own migrated permission shape
// (docs/plans/2026-09-22-descriptor-channel-split.md P2b): an ASK-direction
// event, channel-split, reply: staying flat on the event (see ChannelBlock's
// own doc comment on why it is not per-block).
const channelSplitPermission = `  permission:
    required: [session_id, tool_name]
    timeout_seconds: 270
    api:
      ask: approval/request
      map: { prompt_id: "$rpc.id" }
    hooks:
      ask: PermissionRequest
      map: { prompt_id: prompt_id }
    reply: { allow: '{"decision":"approved"}', deny: '{"decision":"denied"}' }
`

func withChannelSplitAskEvent(base string) string {
	return strings.Replace(base, legacyPermission, channelSplitPermission, 1)
}

func TestParseV3_AskChannelBlockEventParsesAlongsideLegacyEvents(t *testing.T) {
	d := mustParse(t, withChannelSplitAskEvent(minimalV3))

	ev := d.Events["permission"]
	if ev.API == nil || ev.Hooks == nil {
		t.Fatalf("permission must carry both channel blocks: api=%v hooks=%v", ev.API, ev.Hooks)
	}
	if got := ev.API.Ask.Name(); got != "approval/request" {
		t.Errorf("permission.api.ask = %q", got)
	}
	if got := ev.Hooks.Ask.Name(); got != "PermissionRequest" {
		t.Errorf("permission.hooks.ask = %q", got)
	}
	if got := ev.Reply["allow"]; got == "" {
		t.Error("permission.reply.allow is empty — reply: must stay flat across the split")
	}

	// The other legacy events in the same descriptor are unaffected.
	if got := d.Events["session_start"].In.Name(); got != "thread/started" {
		t.Errorf("session_start.in = %q (legacy events must still parse unchanged)", got)
	}
}

// The whole point of the split, for an ask event too: AnswerFor must resolve
// the reply channel from the channel-scoped shape exactly as it did for the
// flat one.
func TestParseV3_AnAskChannelSplitEventIsAnswerable(t *testing.T) {
	d := mustParse(t, withChannelSplitAskEvent(minimalV3))

	got, ok := d.AnswerFor("permission")
	if !ok {
		t.Fatal("a channel-split ask event with reply: templates must be answerable")
	}
	if got.TimeoutSeconds != 270 || got.Responses["allow"] == "" {
		t.Errorf("AnswerFor(permission) = %+v", got)
	}
}

// A block naming BOTH in: and ask: is ambiguous — mirrors the flat-vs-channel
// mixing rejections above, one level down.
func TestParseV3_RejectsAChannelBlockDeclaringBothInAndAsk(t *testing.T) {
	bad := strings.Replace(withChannelSplitAskEvent(minimalV3),
		"    api:\n      ask: approval/request\n",
		"    api:\n      ask: approval/request\n      in: approval/request\n", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("a channel block declaring both in: and ask: is ambiguous and must be rejected")
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
