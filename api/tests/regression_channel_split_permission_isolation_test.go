//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const channelSplitPermissionStubProviderDescriptorYAML = `id: channelsplitpermstub
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
  permission:
    required: [session_id, tool_name]
    timeout_seconds: 5
    api:
      ask: approval/request
      map:
        session_id: threadId
        message:    reason
        tool_name:  tool
    hooks:
      ask: PermissionRequest
      map:
        session_id: session_id
        message:    message
        tool_name:  tool_name
    reply:
      allow: '{"decision":"accept"}'
      deny:  '{"decision":"decline"}'
`

func permissionChannelSplitChoices(t *testing.T, h *harness, chatID string) []ledger.Ask {
	t.Helper()
	return rowsOf(t, chatRows(t, h, chatID), ledger.KindAsk, func(ledger.Ask) bool { return true })
}

func TestRegression_HooksDeliveryOfAChannelSplitAskEventCannotReadAPIShapedFields(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "channelsplitpermstub", channelSplitPermissionStubProviderDescriptorYAML)
	ws := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, ws, "channelsplitpermstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(ws), "channelsplitpermstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"do a thing"}`)
	h.Quiesce()

	askHook(t.Context(), t, h, repoBase(ws), "channelsplitpermstub", runnerID, "permission",
		`{"session_id":"sess-1","message":"allow this?","tool_name":"Bash"}`)
	heldChoice(t, h, chatID)

	choices := permissionChannelSplitChoices(t, h, chatID)
	require.Len(t, choices, 1, "the genuine hooks-shaped permission must raise one card")
	assert.Equal(t, "Bash", choices[0].ToolName, "resolved via the hooks: block's tool_name: tool_name")

	resp := stubLaunch(t, h, "channelsplitpermstub", runnerID).PostHook(t, h.url+repoBase(ws), "channelsplitpermstub", "permission", //nolint:bodyclose // PostHook closes the body and returns a buffered copy
		[]byte(`{"threadId":"attacker-thread","reason":"attacker reason","tool":"ATTACKER-TOOL-LEAKED"}`))
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	h.Quiesce()

	choices = permissionChannelSplitChoices(t, h, chatID)
	require.Len(t, choices, 1, "the api-shaped delivery is missing its required session_id/tool_name on "+
		"the hooks: block — required: (design spec 2.3) must reject it outright, not raise a card under "+
		"a fallback identity")
	assert.Equal(t, "Bash", choices[0].ToolName, "the genuine hooks-shaped card must be the only one raised — "+
		"none of the attacker's tool name ever reached the choice desk")
}
