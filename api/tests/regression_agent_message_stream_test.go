//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// EVERY MESSAGE OF A TURN, NOT JUST THE LAST ONE.
//
// The defect this file pins was reproduced by a user in one sentence: they asked
// claude to "send me a message, wait 30 seconds, then send another". claude did
// exactly that, and Crowbar's record held ONE assistant turn — the second
// message. The first was gone.
//
// The cause is that a turn's terminating hook reports a single message. claude's
// Stop carries `last_assistant_message`, which is literally the last one, so
// anything said earlier in the turn was never ingested by anything. That record is
// what get_chat_log serves to sibling agents, so the loss was not cosmetic:
// another agent read this conversation with the middle of every turn missing.
//
// It was FIRST fixed by reading the provider's own JSONL transcript, and that fix
// has been removed. A provider's private file format is not a contract, it changes
// without notice, and reaching into it is not something to ship. The source is now
// the provider's own STREAMING HOOK — declared, outside-facing, and measured
// byte-identical to what the terminating hook reports for the message they share.
//
// These tests drive the WHOLE stack over HTTP: the same /agent/hooks route a
// vendor CLI's relay posts to, and the same /agent/chats/:id/messages route the
// chat pane reads from.

// streamStubProviderDescriptorYAML is livestub plus a message_delta declaration,
// shaped exactly like claude's MessageDisplay: an increment, a message identity to
// group it by, and a contiguous index to order it by.
//
// It spawns `cat`, which holds its PTY open so the runner stays live across the
// whole turn.
const streamStubProviderDescriptorYAML = `id: streamstub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
    message_delta:
      session_id: session_id
      turn_id:    turn_id
      message_id: message_id
      index:      index
      final:      final
      text:       delta
    turn_failed:
      session_id: session_id
      reason:     error
      detail:     error_details
      message:    last_assistant_message
    tool_pre:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: tool_input.command
    tool_post:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
`

// quietStubProviderDescriptorYAML is the SAME descriptor with the streaming block
// removed, and it exists for one assertion: a provider that declares nothing must
// behave exactly as it did before any of this was written. codex is that provider
// in production — it declares eleven hook events and none of them streams.
const quietStubProviderDescriptorYAML = `id: quietstub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
    tool_pre:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: tool_input.command
    tool_post:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
`

func writeProviderDescriptor(t *testing.T, h *harness, id, body string) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o644))
}

func createStubChat(t *testing.T, h *harness, imported importedRepo, provider string) (chatID, runnerID string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": provider}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	detail := getAgentChat(t, h, wsBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	return created.ID, detail.LiveRunnerID
}

// postProviderHook is postAgentHook with the provider named, because these tests
// run two stub providers side by side to contrast their behaviour.
func postProviderHook(
	t *testing.T,
	h *harness,
	imported importedRepo,
	provider, segID, event, payload string,
) {
	t.Helper()
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": provider, "event": event, "payload_raw": payload,
	}, http.StatusAccepted).Body.Close()
}

type recordedMessage struct {
	Sequence int    `json:"sequence"`
	TurnID   string `json:"turnId"`
	Role     string `json:"role"`
	Text     string `json:"text"`
}

// readRecordedMessages reads a chat's conversation through the SAME route the
// chat pane reads, so what these tests assert on is what a user is shown.
func readRecordedMessages(t *testing.T, h *harness, imported importedRepo, chatID string) []recordedMessage {
	t.Helper()
	var page struct {
		Items []recordedMessage `json:"items"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+chatID+"/messages?limit=200", &page)
	return page.Items
}

func assistantTexts(messages []recordedMessage) []string {
	out := []string{}
	for _, m := range messages {
		if m.Role == "assistant" {
			out = append(out, m.Text)
		}
	}
	return out
}

// delta renders one increment the way claude's MessageDisplay does.
func delta(messageID string, index int, final bool, text string) string {
	payload, err := json.Marshal(map[string]any{
		"session_id": "sess-1", "turn_id": "turn-1", "message_id": messageID,
		"index": index, "final": final, "delta": text,
	})
	if err != nil {
		panic(err)
	}
	return string(payload)
}

// TestRegression_EveryAssistantMessageOfATurnIsRecorded is the user's own
// reproduction, driven through the hook route: a turn that speaks, works, and
// speaks again must land BOTH messages, in the order they were said, with the tool
// call attached to the message it followed.
//
// Before the fix the assertion below read one message where two were said. The
// second one — `last_assistant_message` — was the only thing any hook ever
// carried.
func TestRegression_EveryAssistantMessageOfATurnIsRecorded(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"say one, work, say two"}`)
	h.Quiesce()

	// The agent speaks for the first time, in two increments, and finishes.
	post("message_delta", delta("msg-one", 0, false, "MESSAGE "))
	post("message_delta", delta("msg-one", 1, true, "ONE"))
	h.Quiesce()

	// …and then reaches for a tool.
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash",`+
		`"tool_input":{"command":"sleep 30"}}`)
	post("tool_post", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash"}`)
	h.Quiesce()

	// The agent speaks again and ends the turn. THIS is the message the old code
	// recorded, and the only one.
	post("message_delta", delta("msg-two", 0, true, "MESSAGE TWO"))
	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"MESSAGE TWO"}`)
	h.Quiesce()

	messages := readRecordedMessages(t, h, imported, chatID)
	require.Equal(t, []string{"MESSAGE ONE", "MESSAGE TWO"}, assistantTexts(messages),
		"a turn that said two things must be recorded as two assistant messages, in order")

	require.Len(t, messages, 3)
	assert.Equal(t, "user", messages[0].Role)
	assert.Less(t, messages[1].Sequence, messages[2].Sequence,
		"the first message the agent said must sit ABOVE the second in the record")
	assert.NotEqual(t, messages[1].TurnID, messages[2].TurnID,
		"each message needs its own turn id, or the UI cannot say which tools produced which reply")

	var activity struct {
		ToolCalls []struct {
			ID     string `json:"id"`
			TurnID string `json:"turnId"`
		} `json:"toolCalls"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+chatID+"/activity", &activity)
	require.Len(t, activity.ToolCalls, 1, "the tool call must be recorded exactly once")
	assert.Equal(t, messages[2].TurnID, activity.ToolCalls[0].TurnID,
		"the tool call ran in the segment that ended with the second message, and must attach to it")
}

// TestRegression_AMessageIsVisibleBEFOREItsTurnEnds is the half the old fix never
// had, and the reason recording happens at message completion rather than at turn
// end. The agent has said something and is still working; the chat must already
// show it.
func TestRegression_AMessageIsVisibleBEFOREItsTurnEnds(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"speak then work"}`)
	post("message_delta", delta("msg-one", 0, true, "SAID EARLY"))
	h.Quiesce()

	// No turn_stop has been posted. The turn is still open and the agent is still
	// working — and the message must be readable anyway.
	assert.Equal(t, []string{"SAID EARLY"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a completed message must be readable while its turn is still running")
}

// TestRegression_ProviderWithNoStreamingHookRecordsExactlyWhatItAlwaysDid is the
// degradation contract, stated as an equality rather than a hope: the same hooks
// against a descriptor with no message_delta block produce exactly the record the
// old code produced — one assistant message, the terminating hook's own.
func TestRegression_ProviderWithNoStreamingHookRecordsExactlyWhatItAlwaysDid(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "quietstub")

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "quietstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"ONLY MESSAGE"}`)
	h.Quiesce()

	assert.Equal(t, []string{"ONLY MESSAGE"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)))
}

// TestRegression_TheTerminatingHookSupersedesALossyStream.
//
// Hook delivery is unacknowledged, so an increment can simply never arrive. The
// terminating hook carries the last message IN FULL, which makes it a free
// reconciliation pass over exactly the message most at risk — and the provider's
// own copy must win over what Crowbar managed to assemble.
func TestRegression_TheTerminatingHookSupersedesALossyStream(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	// Index 1 never arrives — the middle of the message is simply lost.
	post("message_delta", delta("msg-one", 0, false, "THE BEGINNING "))
	post("message_delta", delta("msg-one", 2, true, "AND THE END"))
	post("turn_stop",
		`{"session_id":"sess-1","last_assistant_message":"THE BEGINNING AND THE MIDDLE AND THE END"}`)
	h.Quiesce()

	assert.Equal(t, []string{"THE BEGINNING AND THE MIDDLE AND THE END"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"the provider's own complete copy must replace what the stream lost")
}

// A message recorded when it completed must not be recorded a SECOND time when
// the turn ends. The terminating hook restates it, and appending it again would
// double every final reply.
func TestRegression_AStreamedReplyIsNotRecordedTwice(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	post("message_delta", delta("msg-one", 0, true, "SAID ONCE"))
	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"SAID ONCE"}`)
	h.Quiesce()

	assert.Equal(t, []string{"SAID ONCE"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)))
}

// TestRegression_CrowbarDeclaresNoProviderTranscript is the rule itself, pinned
// where it can be broken.
//
// Crowbar may consume a provider's OUTSIDE surface only: declared hooks, documented
// flags, config passed at spawn. It may not read the provider's own files — not
// even read-only, and not even when the provider hands over the path. A private
// file format is not a contract, and shipping a read of one is not an option. This
// fails the moment a descriptor grows a transcript declaration back.
func TestRegression_CrowbarDeclaresNoProviderTranscript(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	descriptors := filepath.Join(filepath.Dir(filepath.Dir(thisFile)),
		"internal", "engine", "agents", "internal", "descriptor", "descriptors")

	entries, err := os.ReadDir(descriptors)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(descriptors, entry.Name()))
		require.NoError(t, err)
		assert.NotContainsf(t, string(body), "\ntranscript:",
			"%s declares a provider transcript read; Crowbar consumes the outside surface only",
			entry.Name())
	}
}
