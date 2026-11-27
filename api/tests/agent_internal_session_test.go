//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

const memStubProviderDescriptorYAML = `id: memstub
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
    require_payload_fields: [transcript_path]
`

func writeMemStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	writeProviderDescriptor(t, h, "memstub", memStubProviderDescriptorYAML)
}

func createMemStubChat(t *testing.T, h *harness, imported importedRepo) (chatID, runnerID string) {
	t.Helper()
	return createStubChat(t, h, imported, "memstub")
}

const (
	realStart  = `{"session_id":"019fafae-f27f-7d00-bd23-3bcbce0ece1f","transcript_path":"/h/.codex/sessions/2026/07/29/rollout-019fafae.jsonl","cwd":"/ws","hook_event_name":"SessionStart","model":"gpt-5.6-sol","permission_mode":"default","source":"startup"}`
	realPrompt = `{"session_id":"019fafae-f27f-7d00-bd23-3bcbce0ece1f","turn_id":"019fafaf-38cf","transcript_path":"/h/.codex/sessions/2026/07/29/rollout-019fafae.jsonl","cwd":"/ws","hook_event_name":"UserPromptSubmit","model":"gpt-5.6-sol","permission_mode":"default","prompt":"THE-USERS-OWN-QUESTION"}`

	memoryStart  = `{"session_id":"019fafaf-4f2c-7551-806e-eda96d1cefed","transcript_path":null,"cwd":"/h/.codex/memories","hook_event_name":"SessionStart","model":"gpt-5.6-terra","permission_mode":"bypassPermissions","source":"startup"}`
	memoryPrompt = `{"session_id":"019fafaf-4f2c-7551-806e-eda96d1cefed","turn_id":"019fafaf-4f54","transcript_path":null,"cwd":"/h/.codex/memories","hook_event_name":"UserPromptSubmit","model":"gpt-5.6-terra","permission_mode":"bypassPermissions","prompt":"MEMORY-WRITING-AGENT-PHASE-2-CONSOLIDATION"}`
	memoryStop   = `{"session_id":"019fafaf-4f2c-7551-806e-eda96d1cefed","turn_id":"019fafaf-4f54","transcript_path":null,"cwd":"/h/.codex/memories","hook_event_name":"Stop","model":"gpt-5.6-terra","permission_mode":"bypassPermissions","stop_hook_active":false,"last_assistant_message":"MEMORY-AGENT-REPLY"}`
)

const (
	realSessionID   = "019fafae-f27f-7d00-bd23-3bcbce0ece1f"
	memorySessionID = "019fafaf-4f2c-7551-806e-eda96d1cefed"
)

func TestRegression_InternalProviderSessionDoesNotStealTheChat(t *testing.T) {
	h := newHarness(t)
	writeMemStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := recordSidebar(t, h)

	chat, runner := createMemStubChat(t, h, ws)

	postHook(t, h, repoBase(ws), "memstub", runner, "session_start", realStart)
	frames.awaitConversation(chat, realSessionID)
	postHook(t, h, repoBase(ws), "memstub", runner, "user_prompt", realPrompt)
	frames.awaitTurnStart(chat)
	h.Quiesce()

	require.Equal(t, runner, getAgentChat(t, h, base, chat).LiveRunnerID,
		"precondition: the user's chat has its CLI on it")
	require.Equal(t, []string{realSessionID}, getAgentChat(t, h, base, chat).sessionIDs(),
		"precondition: the chat hosts the user's conversation and only that")

	postHook(t, h, repoBase(ws), "memstub", runner, "session_start", memoryStart)
	postHook(t, h, repoBase(ws), "memstub", runner, "user_prompt", memoryPrompt)
	postHook(t, h, repoBase(ws), "memstub", runner, "turn_stop", memoryStop)
	h.Quiesce()

	var chats []agentChatDTO
	h.get(base+"/chats", &chats)
	chats = conversationsOnly(chats)
	require.Len(t, chats, 1,
		"an internal provider session must not mint a chat: the user opened one conversation and must "+
			"see one chat, not a second one they never asked for")
	require.Equal(t, chat, chats[0].ID)

	after := getAgentChat(t, h, base, chat)
	assert.Equal(t, runner, after.LiveRunnerID,
		"the runner must still be placed on the user's chat: the CLI on screen never moved, so Crowbar "+
			"moving underneath it is the model disagreeing with the process")
	assert.Equal(t, []string{realSessionID}, after.sessionIDs(),
		"the internal session must never be recorded as a conversation this chat hosted — it has no "+
			"rollout, so resuming it would ask the CLI for a thread that does not exist")
	assert.NotContains(t, after.sessionIDs(), memorySessionID)

	handoff := chatHandoff(t, h, base, chat)
	assert.Contains(t, handoff, "THE-USERS-OWN-QUESTION", "precondition: the user's turn IS in the ledger")
	assert.NotContains(t, handoff, "MEMORY-WRITING-AGENT-PHASE-2-CONSOLIDATION",
		"the internal session's prompt must never be filed as something the user said")
	assert.NotContains(t, handoff, "MEMORY-AGENT-REPLY",
		"nor its answer as something the user's CLI replied")
	assert.Equal(t, "THE-USERS-OWN-QUESTION", after.Title,
		"the title is derived from the same hook, so it must follow the same routing")

	postHook(t, h, repoBase(ws), "memstub", runner, "turn_stop",
		`{"session_id":"`+realSessionID+`","transcript_path":"/h/rollout-019fafae.jsonl","last_assistant_message":"THE-USERS-OWN-ANSWER"}`)
	frames.awaitTurnEnd(chat)
	h.Quiesce()
	assert.Contains(t, chatHandoff(t, h, base, chat), "THE-USERS-OWN-ANSWER",
		"the user's conversation must carry on in the chat it was always in")

	for _, row := range frames.liveRows() {
		if row.Kind == string(stream.KindChatEntity) {
			assert.Equal(t, chat, row.ID, "no chat other than the user's may appear on the wire at all; row: %v", row)
		}
	}
}

func TestRegression_InternalSessionHooksAreDroppedNotFailed(t *testing.T) {
	h := newHarness(t)
	writeMemStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)

	_, runner := createMemStubChat(t, h, ws)

	for _, ev := range []struct{ event, payload string }{
		{"session_start", memoryStart},
		{"user_prompt", memoryPrompt},
		{"turn_stop", memoryStop},
	} {
		postHook(t, h, repoBase(ws), "memstub", runner, ev.event, ev.payload)
	}
}
