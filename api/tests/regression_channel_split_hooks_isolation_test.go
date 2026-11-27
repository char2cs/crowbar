//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const channelSplitStubProviderDescriptorYAML = `id: channelsplitstub
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

func toolCalls(t *testing.T, h *harness, chatID string) []ledger.ToolCall {
	t.Helper()
	seen := map[string]bool{}
	return rowsOf(t, chatRows(t, h, chatID), ledger.KindToolCall, func(b ledger.ToolCall) bool {
		first := !seen[b.ToolID]
		seen[b.ToolID] = true
		return first
	})
}

func TestRegression_HooksDeliveryOfAChannelSplitEventCannotReadAPIShapedFields(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "channelsplitstub", channelSplitStubProviderDescriptorYAML)
	ws := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, ws, "channelsplitstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(ws), "channelsplitstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"do a thing"}`)
	h.Quiesce()

	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-hooks-1","tool_name":"Bash",`+
		`"tool_input":{"command":"echo hi"}}`)
	h.Quiesce()

	calls := toolCalls(t, h, chatID)
	require.Len(t, calls, 1, "the genuine hooks-shaped tool_pre must record one tool call")
	assert.Equal(t, "tool-hooks-1", calls[0].ToolID, "resolved via the hooks: block's tool_id: tool_use_id")
	assert.Equal(t, "Bash", calls[0].Name, "resolved via the hooks: block's tool_name: tool_name")
	assert.Equal(t, "echo hi", calls[0].Target, "resolved via the hooks: block's tool_target: tool_input.command")

	resp := stubLaunch(t, h, "channelsplitstub", runnerID).PostHook(t, h.url+repoBase(ws), "channelsplitstub", "tool_pre", //nolint:bodyclose // PostHook closes the body and returns a buffered copy
		[]byte(`{"threadId":"attacker-thread","item":{"type":"commandExecution",`+
			`"id":"attacker-tool-id","command":"ATTACKER-COMMAND-LEAKED"}}`))
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	h.Quiesce()

	calls = toolCalls(t, h, chatID)
	require.Len(t, calls, 1, "the api-shaped delivery is missing its required session_id/tool_id/"+
		"tool_name on the hooks: block — required: (design spec 2.3) must reject it outright, "+
		"not record it under a fallback identity")
	assert.Equal(t, "tool-hooks-1", calls[0].ToolID, "the genuine hooks-shaped call must be the only one recorded")
	assert.Equal(t, "Bash", calls[0].Name)
	assert.Equal(t, "echo hi", calls[0].Target,
		"none of the attacker's id/type/command ever reached the ledger")
}
