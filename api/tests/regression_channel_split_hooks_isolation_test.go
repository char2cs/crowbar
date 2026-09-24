//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelSplitStubProviderDescriptorYAML mirrors codex.yaml's OWN migrated
// tool_pre shape (descriptors-v3/codex.yaml, since docs/plans/2026-09-22-
// descriptor-channel-split.md P3): one canonical event, an api: block and a
// hooks: block with DIFFERENT wire names and DIFFERENT field paths for the
// same conversational fact. session_start/user_prompt/turn_stop stay
// flat/hooks-only — only tool_pre is under test here.
const channelSplitStubProviderDescriptorYAML = `id: channelsplitstub
spawn:
  cmd: "cat"
  interactive_required: true
runtime:
  transport: hooks
  hooks:
    format: json
events:
  session_start:
    in: session_start
    map: { session_id: session_id }
  user_prompt:
    in: user_prompt
    map: { message: prompt }
  turn_stop:
    in: turn_stop
    map: { session_id: session_id, message: last_assistant_message }
  tool_pre:
    required: [session_id, tool_id, tool_name]
    api:
      in: item/started
      when: { item.type: { any_of: [commandExecution, fileChange] } }
      map:
        session_id:  threadId
        tool_id:     item.id
        tool_name:   item.type
        tool_target: item.command
    hooks:
      in: PreToolUse
      map:
        session_id:  session_id
        tool_id:     tool_use_id
        tool_name:   tool_name
        tool_target: tool_input.command
`

type channelSplitActivityCall struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Target string `json:"target"`
}

func channelSplitActivity(t *testing.T, h *harness, ws importedRepo, chatID string) []channelSplitActivityCall {
	t.Helper()
	var out struct {
		ToolCalls []channelSplitActivityCall `json:"toolCalls"`
	}
	h.get(repoBase(ws)+"/chats/"+chatID+"/activity", &out)
	return out.ToolCalls
}

// TestRegression_HooksDeliveryOfAChannelSplitEventCannotReadAPIShapedFields is
// the chat-theft CLASS (docs/plans/2026-09-22-descriptor-channel-split.md 1.1),
// proven at the black-box HTTP level against a descriptor using the SAME
// api:/hooks: shape codex.yaml itself now ships for tool_pre (that plan's P3).
//
// POST /chats/hooks always delivers on the HOOKS channel — turn/ingest.go's
// channelFor: "an HTTP hook relay POST marks nothing, so the zero value here
// is hooks" — regardless of what shape the BODY happens to carry. So a
// payload shaped for the OTHER channel (api: threadId/item.*, exactly what a
// real item/started notification carries) posted to this endpoint is exactly
// the foreign-shape delivery the channel split exists to refuse: before the
// split, the single flat map's `||` cross-shape fallback would have read
// straight through it (this is the same mechanism, at the wire-payload
// level, that let codex's internal memory-consolidation session steal a
// user's chat — see TestRegression_InternalProviderSessionDoesNotStealTheChat
// for that identity-level account; this test pins the field-resolution level
// underneath it).
func TestRegression_HooksDeliveryOfAChannelSplitEventCannotReadAPIShapedFields(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "channelsplitstub", channelSplitStubProviderDescriptorYAML)
	ws := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, ws, "channelsplitstub")

	post := func(event, payload string) {
		postProviderHook(t, h, ws, "channelsplitstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"do a thing"}`)
	h.Quiesce()

	// The GENUINE hooks-shaped delivery: resolves through the hooks: block.
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-hooks-1","tool_name":"Bash",`+
		`"tool_input":{"command":"echo hi"}}`)
	h.Quiesce()

	calls := channelSplitActivity(t, h, ws, chatID)
	require.Len(t, calls, 1, "the genuine hooks-shaped tool_pre must record one tool call")
	assert.Equal(t, "tool-hooks-1", calls[0].ID, "resolved via the hooks: block's tool_id: tool_use_id")
	assert.Equal(t, "Bash", calls[0].Name, "resolved via the hooks: block's tool_name: tool_name")
	assert.Equal(t, "echo hi", calls[0].Target, "resolved via the hooks: block's tool_target: tool_input.command")

	// THE ATTACK: an api-SHAPED tool_pre body posted to the SAME /chats/hooks
	// endpoint. If channel selection degenerated back into reading both
	// shapes at once — the old || cross-shape fallback this design replaces
	// — this would resolve the attacker's own id/type/command.
	//
	// required: (design spec 2.3, P5) now makes this a HARDER isolation than
	// "record it anonymously": tool_pre's own session_id/tool_id/tool_name
	// are required, none of the hooks: block's paths (session_id/
	// tool_use_id/tool_name) resolve against this api-shaped body, so the
	// delivery is REJECTED outright (RequiredFieldError) rather than
	// recorded under a fallback identity — strictly stronger than before:
	// nothing attacker-controlled reaches the ledger AT ALL, not even a
	// nameless placeholder row.
	post("tool_pre", `{"threadId":"attacker-thread","item":{"type":"commandExecution",`+
		`"id":"attacker-tool-id","command":"ATTACKER-COMMAND-LEAKED"}}`)
	h.Quiesce()

	calls = channelSplitActivity(t, h, ws, chatID)
	require.Len(t, calls, 1, "the api-shaped delivery is missing its required session_id/tool_id/"+
		"tool_name on the hooks: block — required: (design spec 2.3) must reject it outright, "+
		"not record it under a fallback identity")
	assert.Equal(t, "tool-hooks-1", calls[0].ID, "the genuine hooks-shaped call must be the only one recorded")
	assert.Equal(t, "Bash", calls[0].Name)
	assert.Equal(t, "echo hi", calls[0].Target,
		"none of the attacker's id/type/command ever reached the ledger")
}
