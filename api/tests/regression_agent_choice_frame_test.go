//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A prompt the agent is blocked on rides the chat feed as a `choice` frame
// carrying the whole set, so a view that was hidden when it opened needs no read
// of its own to show it.

const answerStubProviderDescriptorYAML = `id: answerstub
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
  permission:
    ask: permission
    timeout_seconds: 5
    map:
      session_id: session_id
      prompt_id:  prompt_id
      message:    tool_name
      tool_name:  tool_name
      tool_target: tool_input.command
      tool_input: tool_input
    reply:
      allow: '{"decision":{"behavior":"allow"}}'
      deny: '{"decision":{"behavior":"deny","message":{reason_json}}}'
runtime:
  transport: hooks
  hooks:
    format: json
`

type choiceFrameChat struct {
	h        *harness
	imported importedRepo
	chatID   string
	runnerID string
	// held is each ask's held hook, by choice id: its CLI's answer arrives on it.
	held map[string]<-chan heldHook
}

func newChoiceFrameChat(t *testing.T, descriptor string) choiceFrameChat {
	t.Helper()
	h := newHarness(t)
	writeProviderDescriptor(t, h, "answerstub", descriptor)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "answerstub")
	postHook(t, h, repoBase(imported), "answerstub", runnerID, "user_prompt", `{"prompt":"go"}`)
	h.Quiesce()
	return choiceFrameChat{h: h, imported: imported, chatID: chatID, runnerID: runnerID, held: map[string]<-chan heldHook{}}
}

func (c choiceFrameChat) ask(t *testing.T, promptID string) (choiceID string) {
	t.Helper()
	return c.askAs(t.Context(), t, "s1", promptID)
}

// askAs opens a permission ask from session; the daemon holds its hook open
// until it is decided, or until ctx hangs it up.
func (c choiceFrameChat) askAs(ctx context.Context, t *testing.T, session, promptID string) (choiceID string) {
	t.Helper()
	seen := make([]string, 0, len(c.held))
	for id := range c.held {
		seen = append(seen, id)
	}
	held := askHook(ctx, t, c.h, repoBase(c.imported), "answerstub", c.runnerID, "permission",
		`{"session_id":"`+session+`","prompt_id":"`+promptID+`","tool_name":"Bash",`+
			`"tool_input":{"command":"touch `+promptID+`"}}`)
	choiceID = heldChoice(t, c.h, repoBase(c.imported), c.chatID, seen...)
	c.held[choiceID] = held
	return choiceID
}

// reply is what the CLI held on choiceID was answered.
func (c choiceFrameChat) reply(t *testing.T, choiceID string) string {
	t.Helper()
	got := <-c.held[choiceID]
	require.NoError(t, got.err)
	require.Equal(t, http.StatusOK, got.status, got.body)
	return got.body
}

func (c choiceFrameChat) dial() *websocket.Conn {
	return c.h.dial(repoBase(c.imported) + "/chats/ws")
}

// choiceFrame matches the chat's `choice` frame whose prompts satisfy want.
func (c choiceFrameChat) choiceFrame(want func([]map[string]any) bool) func(map[string]any) bool {
	return func(m map[string]any) bool {
		if m["chatId"] != c.chatID || m["kind"] != "choice" {
			return false
		}
		raw, _ := m["choices"].([]any)
		choices := make([]map[string]any, 0, len(raw))
		for _, r := range raw {
			choice, _ := r.(map[string]any)
			choices = append(choices, choice)
		}
		return want(choices)
	}
}

func choiceIDs(choices []map[string]any) []string {
	ids := make([]string, 0, len(choices))
	for _, choice := range choices {
		id, _ := choice["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func allowOptionID(t *testing.T, c choiceFrameChat, choiceID string) string {
	t.Helper()
	var choices []struct {
		ID      string `json:"id"`
		Options []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"options"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/choices", &choices)
	for _, choice := range choices {
		if choice.ID != choiceID {
			continue
		}
		for _, option := range choice.Options {
			if option.Kind == "allow" {
				return option.ID
			}
		}
	}
	require.FailNow(t, "no allow option for "+choiceID)
	return ""
}

func (c choiceFrameChat) answer(t *testing.T, choiceID string) {
	t.Helper()
	c.h.post(repoBase(c.imported)+"/chats/"+c.chatID+"/choices/"+choiceID+"/answer",
		map[string]any{"optionIds": []string{allowOptionID(t, c, choiceID)}}, http.StatusOK, nil)
}

func TestRegression_AChoiceOpensAndClearsOverTheChatSocket(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)
	conn := c.dial()

	choiceID := c.ask(t, "p1")
	frame := readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["id"] == choiceID &&
			choices[0]["pending"] == true && choices[0]["answerable"] == true
	}))
	require.Equal(t, "choice", frame["kind"])

	c.answer(t, choiceID)
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool { return len(choices) == 0 }))
}

func TestRegression_AChoiceOpenedBeforeASocketConnectedIsReplayedToIt(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)
	choiceID := c.ask(t, "p1")
	c.h.Quiesce()

	late := c.dial()
	readUntil(t, late, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["id"] == choiceID && choices[0]["answerable"] == true
	}))
}

func TestRegression_SeveralChoicesRideOneFrameAndEachAnswerShrinksIt(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)
	conn := c.dial()

	first := c.ask(t, "p1")
	second := c.ask(t, "p2")
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 2
	}))

	c.answer(t, first)
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		ids := choiceIDs(choices)
		return len(ids) == 1 && ids[0] == second
	}))
}

// A CLI that hangs up its held ask has asked the question itself: the choice
// closes as proceeded, and no card is left.
func TestRegression_AHeldAskWhoseCLIHungUpIsClosedAsProceeded(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)
	conn := c.dial()

	hangUp, cancel := context.WithCancel(t.Context())
	choiceID := c.askAs(hangUp, t, "s1", "p1")
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["answerable"] == true
	}))

	cancel()
	require.Error(t, (<-c.held[choiceID]).err, "the CLI hung up")
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool { return len(choices) == 0 }))
	var activity struct {
		Choices []struct{ ID, Resolution string } `json:"choices"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/activity", &activity)
	require.Len(t, activity.Choices, 1)
	require.Equal(t, choiceID, activity.Choices[0].ID)
	require.Equal(t, "proceeded", activity.Choices[0].Resolution, "its CLI went on without Crowbar's answer")
}

// The CLI holding an ask receives the bytes Crowbar's record says it was given.
func TestRegression_AnAnsweredAskRepliesOnItsHeldHook(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)
	choiceID := c.ask(t, "p1")

	c.answer(t, choiceID)
	assert.JSONEq(t, `{"decision":{"behavior":"allow"}}`, c.reply(t, choiceID))
}
