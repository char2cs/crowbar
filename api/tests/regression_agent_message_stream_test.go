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

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

const streamStubProviderDescriptorYAML = `id: streamstub
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
  message_delta:
    in: message_delta
    map:
      session_id: session_id
      turn_id:    turn_id
      message_id: message_id
      index:      index
      final:      final
      text:       delta
  turn_failed:
    in: turn_failed
    map:
      session_id: session_id
      reason:     error
      detail:     error_details
      message:    last_assistant_message
  tool_pre:
    in: tool_pre
    map:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: tool_input.command
  tool_post:
    in: tool_post
    map:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
runtime:
  transport: hooks
  hooks:
    format: json
`

const quietStubProviderDescriptorYAML = `id: quietstub
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
  tool_pre:
    in: tool_pre
    map:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: tool_input.command
  tool_post:
    in: tool_post
    map:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
runtime:
  transport: hooks
  hooks:
    format: json
`

func createStubChat(t *testing.T, h *harness, imported importedRepo, provider string) (chatID, runnerID string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": provider, "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	return created.ID, detail.LiveRunnerID
}

type recordedMessage = kit.Message

// readRecordedMessages is what chatID's feed rows say, as a client shows it.
func readRecordedMessages(t *testing.T, h *harness, _ importedRepo, chatID string) []recordedMessage {
	t.Helper()
	return kit.Said(ledgerRows(t, h, chatID))
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
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
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
	assert.Less(t, messages[1].Seq, messages[2].Seq,
		"the first message the agent said must sit ABOVE the second in the record")
	assert.NotEqual(t, messages[1].ID, messages[2].ID, "each message keeps its own id")

	require.Len(t, toolCalls(t, h, chatID), 1, "the tool call must be recorded exactly once")
	ran := seqOf(t, chatRows(t, h, chatID), ledger.KindToolCall, func(b ledger.ToolCall) bool { return b.ToolID == "tool-1" })
	assert.Greater(t, ran, messages[1].Seq, "the tool ran after the first message")
	assert.Less(t, ran, messages[2].Seq, "and before the second")
}

func TestRegression_AMessageIsVisibleBEFOREItsTurnEnds(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"speak then work"}`)
	post("message_delta", delta("msg-one", 0, true, "SAID EARLY"))
	h.Quiesce()

	assert.Equal(t, []string{"SAID EARLY"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a completed message must be readable while its turn is still running")
}

// A growing message reaches the chat's feed while its turn runs, though it is
// not a settled message yet.
func TestRegression_AGrowingMessageReachesTheFeedBeforeTheTurnEnds(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")
	feed := dialFeed(t, h, chatID, 0)
	untilReady(t, feed)

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"narrate while you work"}`)
	post("message_delta", delta("msg-one", 0, false, "STILL "))
	post("message_delta", delta("msg-one", 1, false, "GROWING"))

	// A numbered chunk rides the feed as its own row the moment it lands.
	var said string
	for said != "STILL GROWING" {
		f := nextFeedFrame(t, feed)
		var chunk ledger.AssistantText
		if f.Row != nil && f.Row.Kind == ledger.KindAssistantText &&
			json.Unmarshal(f.Row.Body, &chunk) == nil && chunk.MessageID == "msg-one" {
			said += chunk.Text
		}
	}
	assert.Empty(t, assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"an unfinished message is not yet said")

	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"STILL GROWING"}`)
	h.Quiesce()

	assert.Equal(t, []string{"STILL GROWING"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"the finished message lands in the ledger once")
}

func TestRegression_ProviderWithNoStreamingHookRecordsExactlyWhatItAlwaysDid(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "quietstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "quietstub", runnerID, event, payload)
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
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
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

// TestRegression_ALossyTerminatingHookNeverTruncatesTheStream is
// TestRegression_TheTerminatingHookSupersedesALossyStream's missing
// counterpart — the two together are the whole reconciliation contract, and
// only one direction had a test. Reported live 2026-09-01: a full,
// multi-paragraph reply was visible while the turn was still streaming, but
// a real provider's own terminating hook reported only the reply's LAST
// paragraph, and closeAssistantTurn's reconciliation silently deleted the
// rest from the persisted transcript on the strength of that shorter report.
func TestRegression_ALossyTerminatingHookNeverTruncatesTheStream(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)

	post("message_delta", delta("msg-one", 0, false, "THE BEGINNING AND THE MIDDLE "))
	post("message_delta", delta("msg-one", 1, true, "AND THE END"))
	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"AND THE END"}`)
	h.Quiesce()

	assert.Equal(t, []string{"THE BEGINNING AND THE MIDDLE AND THE END"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)),
		"a shorter terminating-hook report must never overwrite text the stream already delivered in full")
}

func TestRegression_AStreamedReplyIsNotRecordedTwice(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	post("message_delta", delta("msg-one", 0, true, "SAID ONCE"))
	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"SAID ONCE"}`)
	h.Quiesce()

	assert.Equal(t, []string{"SAID ONCE"},
		assistantTexts(readRecordedMessages(t, h, imported, chatID)))
}

// A turn_stop that beats its own final delta records the reply once: the late
// delta joins the reply its turn_stop recorded, and the fuller text wins.
func TestRegression_AFinalDeltaAfterItsTurnStopJoinsTheReply(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	post("turn_stop", `{"session_id":"sess-1","last_assistant_message":"SAID"}`)
	post("message_delta", delta("msg-one", 0, true, "SAID IN FULL"))
	h.Quiesce()

	assert.Equal(t, []string{"SAID IN FULL"}, assistantTexts(readRecordedMessages(t, h, imported, chatID)))
}

func TestRegression_CrowbarDeclaresNoProviderTranscript(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	descriptors := filepath.Join(filepath.Dir(filepath.Dir(thisFile)),
		"internal", "engine", "agents", "internal", "protocol", "internal", "descriptor", "descriptors-v3")

	entries, err := os.ReadDir(descriptors)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, entry := range entries {
		// descriptors-v3 holds an experimental/ subdirectory alongside the shipped
		// files; the rule applies to the YAML, not the tree.
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(descriptors, entry.Name()))
		require.NoError(t, err)
		assert.NotContainsf(t, string(body), "\ntranscript:",
			"%s declares a provider transcript read; Crowbar consumes the outside surface only",
			entry.Name())
	}
}
