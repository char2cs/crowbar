//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memStubProviderDescriptorYAML is livestub plus the one line this file exists to
// test: hooks.require_payload_fields. It mirrors what codex.yaml declares.
//
// It is a SEPARATE descriptor rather than a change to livestub because livestub's
// job in the other agent tests is to be the provider that declares nothing —
// which is also the fixture that proves this guard is opt-in and cannot have
// changed claude.
const memStubProviderDescriptorYAML = `id: memstub
spawn:
  cmd: "cat"
  interactive_required: true
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
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "memstub.yaml"), []byte(memStubProviderDescriptorYAML), 0o644))
}

// createMemStubChat is createLiveStubChat against the memstub provider.
func createMemStubChat(t *testing.T, h *harness, imported importedRepo) (chatID, runnerID string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "memstub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	return created.ID, detail.LiveRunnerID
}

// postMemStubHook fires one in-PTY hook callback for the memstub provider.
func postMemStubHook(
	t *testing.T,
	h *harness,
	imported importedRepo,
	runnerID, event, payload string,
) {
	t.Helper()
	_ = h.raw(http.MethodPost, repoBase(imported)+"/chats/hooks", map[string]string{
		"segment_id": runnerID, "provider": "memstub", "event": event, "payload_raw": payload,
	}, http.StatusAccepted).Body.Close()
}

// The payloads below are VERBATIM captures from codex 0.146.0 with
// `[features] memories = true`, recorded by standing in for `crowbar hook` on the
// real `-c hooks.*` commands the codex descriptor injects. Only the paths are
// shortened. All five came out of ONE codex process, six seconds apart.
const (
	realStart  = `{"session_id":"019fafae-f27f-7d00-bd23-3bcbce0ece1f","transcript_path":"/h/.codex/sessions/2026/07/29/rollout-019fafae.jsonl","cwd":"/ws","hook_event_name":"SessionStart","model":"gpt-5.6-sol","permission_mode":"default","source":"startup"}`
	realPrompt = `{"session_id":"019fafae-f27f-7d00-bd23-3bcbce0ece1f","turn_id":"019fafaf-38cf","transcript_path":"/h/.codex/sessions/2026/07/29/rollout-019fafae.jsonl","cwd":"/ws","hook_event_name":"UserPromptSubmit","model":"gpt-5.6-sol","permission_mode":"default","prompt":"THE-USERS-OWN-QUESTION"}`

	// source is `startup` here too — identical to a real /new, which is why no move
	// label can separate these from the user's own conversation.
	memoryStart  = `{"session_id":"019fafaf-4f2c-7551-806e-eda96d1cefed","transcript_path":null,"cwd":"/h/.codex/memories","hook_event_name":"SessionStart","model":"gpt-5.6-terra","permission_mode":"bypassPermissions","source":"startup"}`
	memoryPrompt = `{"session_id":"019fafaf-4f2c-7551-806e-eda96d1cefed","turn_id":"019fafaf-4f54","transcript_path":null,"cwd":"/h/.codex/memories","hook_event_name":"UserPromptSubmit","model":"gpt-5.6-terra","permission_mode":"bypassPermissions","prompt":"MEMORY-WRITING-AGENT-PHASE-2-CONSOLIDATION"}`
	memoryStop   = `{"session_id":"019fafaf-4f2c-7551-806e-eda96d1cefed","turn_id":"019fafaf-4f54","transcript_path":null,"cwd":"/h/.codex/memories","hook_event_name":"Stop","model":"gpt-5.6-terra","permission_mode":"bypassPermissions","stop_hook_active":false,"last_assistant_message":"MEMORY-AGENT-REPLY"}`
)

const (
	realSessionID   = "019fafae-f27f-7d00-bd23-3bcbce0ece1f"
	memorySessionID = "019fafaf-4f2c-7551-806e-eda96d1cefed"
)

// TestRegression_InternalProviderSessionDoesNotStealTheChat is the black-box
// contract for the codex-memories bug: a vendor CLI's hooks are not only its
// USER'S hooks, and a hook arriving through them is not proof that it is about the
// conversation on screen.
//
// codex 0.146.0 with memories enabled runs its Memory Writing Agent as an INTERNAL
// session inside the same process. That session inherits the `-c hooks.*` commands
// Crowbar injected, so it fires the whole lifecycle at Crowbar — SessionStart,
// UserPromptSubmit, Stop — carrying a session id nobody has seen and
// `source: startup`, which is byte-identical to the user typing /new.
//
// What that produced, live, was a chat theft with no error anywhere:
//
//  1. the announcement was an unknown conversation under a live runner, so the
//     reducer read it as /new: it MINTED a chat and MOVED the runner into it,
//  2. leaving the user's chat with no CLI on it and its open turn force-closed,
//  3. the internal session's own prompt then titled the phantom chat and went into
//     its ledger as something the USER said,
//  4. and the CLI on screen never moved at all — so the user's next message landed
//     in the phantom chat while the conversation they were looking at was stranded.
//
// Every numbered outcome above is asserted below, against the durable oracles (the
// chat list, the runner's placement, the conversation history, the on-disk ledger)
// rather than against the drop itself — so a regression that finds a NEW way to let
// an internal session through fails here too.
//
// NO TIMING. POST /chats/hooks runs IngestHook synchronously, so a dropped hook has
// finished being dropped by the time its 202 returns; h.Quiesce is the
// read-your-writes barrier (asynx WaitPublish) before the plain REST reads. The
// negative assertions are then made STRICT by a real signal: a genuine turn is
// taken afterwards and its turn_started frame awaited, and WS frames are ordered,
// so any frame the internal session's hooks would have produced necessarily
// arrived before it.
func TestRegression_InternalProviderSessionDoesNotStealTheChat(t *testing.T) {
	h := newHarness(t)
	writeMemStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := recordAgentWS(t, h, base+"/chats/ws")

	chat, runner := createMemStubChat(t, h, ws)

	// The user's own conversation announces itself and takes a turn.
	postMemStubHook(t, h, ws, runner, "session_start", realStart)
	frames.awaitRunner(runner, "session_bound")
	postMemStubHook(t, h, ws, runner, "user_prompt", realPrompt)
	frames.awaitChat(chat, "turn_started")
	h.Quiesce()

	require.Equal(t, runner, getAgentChat(t, h, base, chat).LiveRunnerID,
		"precondition: the user's chat has its CLI on it")
	require.Equal(t, []string{realSessionID}, getAgentChat(t, h, base, chat).sessionIDs(),
		"precondition: the chat hosts the user's conversation and only that")

	// --- the bug: the provider's INTERNAL session runs the full lifecycle at us ---
	postMemStubHook(t, h, ws, runner, "session_start", memoryStart)
	postMemStubHook(t, h, ws, runner, "user_prompt", memoryPrompt)
	postMemStubHook(t, h, ws, runner, "turn_stop", memoryStop)
	h.Quiesce()

	// (1) No phantom chat was minted.
	var chats []agentChatDTO
	h.get(base+"/chats", &chats)
	chats = conversationsOnly(chats)
	require.Len(t, chats, 1,
		"an internal provider session must not mint a chat: the user opened one conversation and must "+
			"see one chat, not a second one they never asked for")
	require.Equal(t, chat, chats[0].ID)

	// (2) The runner never left the user's chat, and the chat never lost its CLI.
	after := getAgentChat(t, h, base, chat)
	assert.Equal(t, runner, after.LiveRunnerID,
		"the runner must still be placed on the user's chat: the CLI on screen never moved, so Crowbar "+
			"moving underneath it is the model disagreeing with the process")
	assert.Equal(t, []string{realSessionID}, after.sessionIDs(),
		"the internal session must never be recorded as a conversation this chat hosted — it has no "+
			"rollout, so resuming it would ask the CLI for a thread that does not exist")
	assert.NotContains(t, after.sessionIDs(), memorySessionID)

	// (3) Nothing the internal session said is in the user's chat, and it did not
	// retitle it. The ledger is the on-disk oracle: it answers after every piece of
	// in-memory state is gone.
	handoff := chatHandoff(t, h, base, chat)
	assert.Contains(t, handoff, "THE-USERS-OWN-QUESTION", "precondition: the user's turn IS in the ledger")
	assert.NotContains(t, handoff, "MEMORY-WRITING-AGENT-PHASE-2-CONSOLIDATION",
		"the internal session's prompt must never be filed as something the user said")
	assert.NotContains(t, handoff, "MEMORY-AGENT-REPLY",
		"nor its answer as something the user's CLI replied")
	assert.Equal(t, "THE-USERS-OWN-QUESTION", after.Title,
		"the title is derived from the same hook, so it must follow the same routing")

	// (4) And the conversation on screen still works: the next real turn lands here,
	// in this chat, on this runner. This is the ordered signal that makes every
	// negative above strict rather than merely un-awaited.
	postMemStubHook(t, h, ws, runner, "turn_stop",
		`{"session_id":"`+realSessionID+`","transcript_path":"/h/rollout-019fafae.jsonl","last_assistant_message":"THE-USERS-OWN-ANSWER"}`)
	frames.awaitChat(chat, "turn_stopped")
	h.Quiesce()
	assert.Contains(t, chatHandoff(t, h, base, chat), "THE-USERS-OWN-ANSWER",
		"the user's conversation must carry on in the chat it was always in")

	// No frame for any chat other than the user's was EVER emitted — the live-signal
	// counterpart of the chat-list assertion, over the whole recorded stream rather
	// than a point read.
	for _, frame := range frames.snapshot() {
		if id, ok := frame["chatId"].(string); ok && id != "" {
			assert.Equal(t, chat, id,
				"no chat other than the user's may appear on the wire at all; frame: %v", frame)
		}
	}
}

// TestRegression_InternalSessionHooksAreDroppedNotFailed pins the other half of the
// contract, and the one that is easy to lose in a later refactor: a hook Crowbar
// declines to act on must still answer the vendor CLI with success.
//
// The hook command runs INSIDE the CLI's own process tree, on its turn. A non-2xx
// (or a hang, or a stderr splat) is something the CLI has to deal with mid-turn, and
// a listener that breaks the thing it is listening to is worse than no listener —
// which is why every drop in IngestHook returns nil rather than an error (spec §4.7).
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
		resp := h.raw(http.MethodPost, repoBase(ws)+"/chats/hooks", map[string]string{
			"segment_id": runner, "provider": "memstub", "event": ev.event, "payload_raw": ev.payload,
		}, http.StatusAccepted)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusAccepted, resp.StatusCode,
			"a dropped %s must be answered 202: the hook runs inside the vendor CLI's turn and an "+
				"error there is the listener breaking the thing it listens to", ev.event)
	}
}
