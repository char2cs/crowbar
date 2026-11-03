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

// What a turn records as the agent's reply, from its stream and its
// terminating hook: a completed message is readable while the turn runs; a
// provider with no stream records the hook's copy; the hook's complete copy
// fills what a lossy stream dropped, yet a shorter hook report never truncates
// a stream delivered in full (a live report once lost all but the last
// paragraph); a streamed reply is recorded once, even when its final delta
// lands after its turn_stop, and the fuller text wins.
func TestRegression_TheReplyIsReconciledFromTheStreamAndTheTerminatingHook(t *testing.T) {
	type hook struct{ event, payload string }
	stop := func(text string) hook {
		return hook{"turn_stop", `{"session_id":"sess-1","last_assistant_message":"` + text + `"}`}
	}
	for _, tc := range []struct {
		name, provider, descriptor string
		hooks                      []hook
		want                       []string
	}{
		{
			"a message is visible before its turn ends", "streamstub", streamStubProviderDescriptorYAML,
			[]hook{{"message_delta", delta("msg-one", 0, true, "SAID EARLY")}},
			[]string{"SAID EARLY"},
		},
		{
			"a provider with no stream records what it always did", "quietstub", quietStubProviderDescriptorYAML,
			[]hook{stop("ONLY MESSAGE")},
			[]string{"ONLY MESSAGE"},
		},
		{
			"the terminating hook supersedes a lossy stream", "streamstub", streamStubProviderDescriptorYAML,
			[]hook{
				{"message_delta", delta("msg-one", 0, false, "THE BEGINNING ")},
				{"message_delta", delta("msg-one", 2, true, "AND THE END")},
				stop("THE BEGINNING AND THE MIDDLE AND THE END"),
			},
			[]string{"THE BEGINNING AND THE MIDDLE AND THE END"},
		},
		{
			"a lossy terminating hook never truncates the stream", "streamstub", streamStubProviderDescriptorYAML,
			[]hook{
				{"message_delta", delta("msg-one", 0, false, "THE BEGINNING AND THE MIDDLE ")},
				{"message_delta", delta("msg-one", 1, true, "AND THE END")},
				stop("AND THE END"),
			},
			[]string{"THE BEGINNING AND THE MIDDLE AND THE END"},
		},
		{
			"a streamed reply is not recorded twice", "streamstub", streamStubProviderDescriptorYAML,
			[]hook{{"message_delta", delta("msg-one", 0, true, "SAID ONCE")}, stop("SAID ONCE")},
			[]string{"SAID ONCE"},
		},
		{
			"a final delta after its turn_stop joins the reply", "streamstub", streamStubProviderDescriptorYAML,
			[]hook{stop("SAID"), {"message_delta", delta("msg-one", 0, true, "SAID IN FULL")}},
			[]string{"SAID IN FULL"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			writeProviderDescriptor(t, h, tc.provider, tc.descriptor)
			imported := importWritableWorkspace(t, h)
			chatID, runnerID := createStubChat(t, h, imported, tc.provider)
			hooks := append([]hook{
				{"session_start", `{"session_id":"sess-1"}`},
				{"user_prompt", `{"session_id":"sess-1","prompt":"hello"}`},
			}, tc.hooks...)
			for _, k := range hooks {
				postHook(t, h, repoBase(imported), tc.provider, runnerID, k.event, k.payload)
			}
			h.Quiesce()

			assert.Equal(t, tc.want, assistantTexts(readRecordedMessages(t, h, imported, chatID)))
		})
	}
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
