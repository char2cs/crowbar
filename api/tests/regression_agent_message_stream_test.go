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

	post("message_delta", delta("msg-one", 0, false, "MESSAGE "))
	post("message_delta", delta("msg-one", 1, true, "ONE"))
	h.Quiesce()

	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash",`+
		`"tool_input":{"command":"sleep 30"}}`)
	post("tool_post", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash"}`)
	h.Quiesce()

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

	assert.Equal(t, []string{"SAID EARLY"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a completed message must be readable while its turn is still running")
}

// TestRegression_AGrowingMessageReachesTheChatSocketBeforeTheTurnEnds is the
// half TestRegression_AMessageIsVisibleBEFOREItsTurnEnds cannot cover: that one
// reads the REST ledger, and the ledger is written by the ingest path whether or
// not anything is wired to the LIVE feed. Live streaming runs through a plain
// callback field on the agent usecase, assigned by the composition root
// (app.New → startTerminalWaitSweep → BroadcastAgentChatMessageDelta). Drop that
// wiring and the pane goes silent until the turn ends while every ledger
// assertion in this file stays green — so this test watches the socket.
//
// BEFORE is proven positively, on the socket's own ordering rather than on an
// absence: readUntil only ever moves FORWARD through the stream, so reaching
// `turn_stopped` having already consumed both partials means those partials were
// earlier on the wire than the end of the turn.
//
// Both partials are NON-FINAL, and the ledger is checked while they are in
// flight: neither "STILL " nor "STILL GROWING" exists anywhere durable at that
// point, so the socket is the only channel that could have carried them.
func TestRegression_AGrowingMessageReachesTheChatSocketBeforeTheTurnEnds(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	// Subscribe BEFORE the chat exists, then take its own `created` frame as the
	// barrier proving this connection is registered — a frame broadcast before the
	// subscriber attached is a frame no amount of waiting recovers.
	conn := h.dial(wsBase(imported) + "/agent/ws/chats")
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")
	chatFrame := func(kind string) func(map[string]any) bool {
		return func(m map[string]any) bool { return m["chatId"] == chatID && m["kind"] == kind }
	}
	readUntil(t, conn, chatFrame("created"))

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"narrate while you work"}`)
	readUntil(t, conn, chatFrame("turn_started"))

	post("message_delta", delta("msg-one", 0, false, "STILL "))
	post("message_delta", delta("msg-one", 1, false, "GROWING"))

	growing := func(text string) func(map[string]any) bool {
		return func(m map[string]any) bool {
			if !chatFrame("message_delta")(m) {
				return false
			}
			message, ok := m["message"].(map[string]any)
			return ok && message["id"] == "msg-one" && message["text"] == text
		}
	}
	readUntil(t, conn, growing("STILL "))
	readUntil(t, conn, growing("STILL GROWING"))

	assert.Empty(t, assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"an unfinished message is a view, not a record — the socket is the only place it exists")

	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"STILL GROWING"}`)
	readUntil(t, conn, chatFrame("turn_stopped"))
	h.Quiesce()

	assert.Equal(t, []string{"STILL GROWING"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"the finished message lands in the ledger once, after the partials the socket already carried")
}

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

	post("message_delta", delta("msg-one", 0, false, "THE BEGINNING "))
	post("message_delta", delta("msg-one", 2, true, "AND THE END"))
	post("turn_stop",
		`{"session_id":"sess-1","last_assistant_message":"THE BEGINNING AND THE MIDDLE AND THE END"}`)
	h.Quiesce()

	assert.Equal(t, []string{"THE BEGINNING AND THE MIDDLE AND THE END"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"the provider's own complete copy must replace what the stream lost")
}

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
