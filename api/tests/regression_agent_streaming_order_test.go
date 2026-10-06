//go:build integration

package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AStreamingMessageCarriesTheMomentItBeganSoToolsAfterItStayBelow
// pins the ordering owner: a provider that records its message only when the
// turn closes leaves the live socket as the sole home of text said BEFORE a
// tool ran. That frame must say when the text began, or a client can only
// guess and draws the tool above the text that preceded it.
func TestRegression_AStreamingMessageCarriesTheMomentItBeganSoToolsAfterItStayBelow(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	conn := h.dial(repoBase(imported) + "/chats/ws")
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")
	chatFrame := func(kind string) func(map[string]any) bool {
		return func(m map[string]any) bool { return m["chatId"] == chatID && m["kind"] == kind }
	}
	readUntil(t, conn, chatFrame("created"))

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"speak then work"}`)
	readUntil(t, conn, chatFrame("turn_started"))

	post("message_delta", delta("msg-one", 0, false, "ABOUT TO RUN "))
	h.Quiesce()
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash",`+
		`"tool_input":{"command":"ls"}}`)
	h.Quiesce()
	post("message_delta", delta("msg-one", 1, false, "A COMMAND"))

	frame := readUntil(t, conn, func(m map[string]any) bool {
		message, ok := m["message"].(map[string]any)
		return chatFrame("message_delta")(m) && ok && message["text"] == "ABOUT TO RUN A COMMAND"
	})
	message, ok := frame["message"].(map[string]any)
	require.True(t, ok)
	startedAt, ok := message["startedAt"].(string)
	require.True(t, ok, "a streaming message frame must carry when its text began")
	began, err := time.Parse(time.RFC3339Nano, startedAt)
	require.NoError(t, err)

	var activity struct {
		ToolCalls []struct {
			StartedAt time.Time `json:"startedAt"`
		} `json:"toolCalls"`
	}
	h.get(repoBase(imported)+"/chats/"+chatID+"/activity", &activity)
	require.Len(t, activity.ToolCalls, 1)
	assert.True(t, began.Before(activity.ToolCalls[0].StartedAt),
		"text that began before the tool ran must be ordered before it, not stamped by later growth")
}
