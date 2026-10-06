//go:build integration

package tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// A prompt the agent is blocked on rides the chat feed as a `choice` frame
// carrying the whole set, so a view that was hidden when it opened needs no read
// of its own to show it.

type choiceFrameChat struct {
	h        *harness
	imported importedRepo
	chatID   string
	runnerID string
}

func newChoiceFrameChat(t *testing.T, descriptor string) choiceFrameChat {
	t.Helper()
	h := newHarness(t)
	writeProviderDescriptor(t, h, "answerstub", descriptor)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "answerstub")
	postAnswerStubHook(t, h, repoBase(imported), runnerID, "", "user_prompt", `{"prompt":"go"}`)
	h.Quiesce()
	return choiceFrameChat{h: h, imported: imported, chatID: chatID, runnerID: runnerID}
}

func (c choiceFrameChat) ask(t *testing.T, promptID string) (deliveryID, choiceID string) {
	t.Helper()
	deliveryID = uuid.NewString()
	ack := postAnswerStubHook(t, c.h, repoBase(c.imported), c.runnerID, deliveryID, "permission",
		`{"session_id":"s1","prompt_id":"`+promptID+`","tool_name":"Bash",`+
			`"tool_input":{"command":"touch `+promptID+`"}}`)
	require.NotNil(t, ack.Await)
	return deliveryID, ack.Await.ChoiceID
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

	_, choiceID := c.ask(t, "p1")
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
	_, choiceID := c.ask(t, "p1")
	c.h.Quiesce()

	late := c.dial()
	readUntil(t, late, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["id"] == choiceID && choices[0]["answerable"] == true
	}))
}

func TestRegression_SeveralChoicesRideOneFrameAndEachAnswerShrinksIt(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)
	conn := c.dial()

	_, first := c.ask(t, "p1")
	_, second := c.ask(t, "p2")
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 2
	}))

	c.answer(t, first)
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		ids := choiceIDs(choices)
		return len(ids) == 1 && ids[0] == second
	}))
}

func TestRegression_AChoiceWhoseRelayExpiredIsPushedAsUnanswerable(t *testing.T) {
	descriptor := strings.Replace(answerStubProviderDescriptorYAML, "timeout_seconds: 5", "timeout_seconds: 1", 1)
	c := newChoiceFrameChat(t, descriptor)
	conn := c.dial()

	deliveryID, choiceID := c.ask(t, "p1")
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["answerable"] == true
	}))

	// The relay waits out its budget and is released with nobody having answered.
	c.h.post("/v0/projects/"+c.imported.projectID+"/repos/"+c.imported.repoID+"/chats/hooks/await",
		map[string]string{"delivery_id": deliveryID}, http.StatusOK, nil)
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["id"] == choiceID &&
			choices[0]["pending"] == true && choices[0]["answerable"] == false
	}))
}
