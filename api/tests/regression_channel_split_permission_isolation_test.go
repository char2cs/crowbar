//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelSplitPermissionStubProviderDescriptorYAML mirrors codex.yaml's OWN
// migrated permission shape (descriptors-v3/codex.yaml, since docs/plans/
// 2026-09-22-descriptor-channel-split.md P2b): one canonical ASK-direction
// event, an api: block and a hooks: block with DIFFERENT wire names and
// DIFFERENT field paths for the same conversational fact. This is
// regression_channel_split_hooks_isolation_test.go's own tool_pre stub,
// with permission added as the ask-direction counterpart under test here.
const channelSplitPermissionStubProviderDescriptorYAML = `id: channelsplitpermstub
spawn:
  cmd: "cat"
  interactive_required: true
runtime:
  transport: api
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

func permissionChannelSplitChoices(t *testing.T, h *harness, ws importedRepo, chatID string) []struct {
	ID       string `json:"id"`
	ToolName string `json:"toolName"`
} {
	t.Helper()
	var out []struct {
		ID       string `json:"id"`
		ToolName string `json:"toolName"`
	}
	h.get(repoBase(ws)+"/chats/"+chatID+"/choices", &out)
	return out
}

// TestRegression_HooksDeliveryOfAChannelSplitAskEventCannotReadAPIShapedFields
// is TestRegression_HooksDeliveryOfAChannelSplitEventCannotReadAPIShapedFields
// own ASK-direction counterpart — the exact gap P3's own report on permission
// flagged: "ChannelBlock only carries in: ... so a dual-shape ASK event has
// no channel-block spelling yet" (codex.yaml, since replaced). POST
// /chats/hooks always delivers on the HOOKS channel (turn/ingest.go's
// channelFor), regardless of what shape the BODY happens to carry, so a
// payload shaped for the api: block posted here is exactly the foreign-shape
// delivery the channel split exists to refuse.
func TestRegression_HooksDeliveryOfAChannelSplitAskEventCannotReadAPIShapedFields(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "channelsplitpermstub", channelSplitPermissionStubProviderDescriptorYAML)
	ws := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, ws, "channelsplitpermstub")

	post := func(event, payload string) {
		postProviderHook(t, h, ws, "channelsplitpermstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"do a thing"}`)
	h.Quiesce()

	// The GENUINE hooks-shaped delivery: resolves through the hooks: block.
	post("permission", `{"session_id":"sess-1","message":"allow this?","tool_name":"Bash"}`)
	h.Quiesce()

	choices := permissionChannelSplitChoices(t, h, ws, chatID)
	require.Len(t, choices, 1, "the genuine hooks-shaped permission must raise one card")
	assert.Equal(t, "Bash", choices[0].ToolName, "resolved via the hooks: block's tool_name: tool_name")

	// THE ATTACK: an api-SHAPED permission body posted to the SAME
	// /chats/hooks endpoint. If channel selection degenerated back into
	// reading both shapes at once — the old || cross-shape fallback this
	// design replaces — this would resolve the attacker's own tool name.
	//
	// required: (design spec 2.3, P5) now makes this a HARDER isolation than
	// "raise it anonymously": permission's own session_id/tool_name are
	// required, neither of the hooks: block's paths (session_id/tool_name)
	// resolve against this api-shaped body, so the delivery is REJECTED
	// outright (RequiredFieldError) rather than raising a card under a
	// fallback identity — strictly stronger than before: nothing
	// attacker-controlled reaches the choice desk at all, not even a
	// nameless card.
	post("permission", `{"threadId":"attacker-thread","reason":"attacker reason","tool":"ATTACKER-TOOL-LEAKED"}`)
	h.Quiesce()

	choices = permissionChannelSplitChoices(t, h, ws, chatID)
	require.Len(t, choices, 1, "the api-shaped delivery is missing its required session_id/tool_name on "+
		"the hooks: block — required: (design spec 2.3) must reject it outright, not raise a card under "+
		"a fallback identity")
	assert.Equal(t, "Bash", choices[0].ToolName, "the genuine hooks-shaped card must be the only one raised — "+
		"none of the attacker's tool name ever reached the choice desk")
}
