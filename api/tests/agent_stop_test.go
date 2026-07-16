//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentStopKeepsChatDormantAndResumable proves the close-a-chat-tab
// route end-to-end: POST .../agent/chats/:id/stop gracefully terminates the chat's
// live vendor CLI but LEAVES THE CHAT — dormant and resumable — rather than deleting
// it. This is the counterpart of Delete (which erases the chat) and the whole point of
// the tab-close behaviour: the process stops, the conversation survives, and reopening
// revives it.
//
// It spawns LIVESTUB (`cat`), which holds its PTY open, so the chat genuinely has a live
// runner to stop; posts a session_start (binding a real conversation) and a turn_stop
// (writing the ledger) so there is a conversation to keep; then stops and asserts the
// chat is present, dormant, and still carries its bound conversation.
func TestRegression_AgentStopKeepsChatDormantAndResumable(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	chatID, runnerID := createLiveStubChat(t, h, imported)

	// A real, resumable conversation: the CLI announced its native session and wrote a
	// turn (a session id is not a conversation until something lands in it).
	postAgentHook(t, h, imported, runnerID, "session_start", `{"session_id":"sess-stop-1"}`)
	postAgentHook(t, h, imported, runnerID, "turn_stop", `{"session_id":"sess-stop-1","last_assistant_message":"hello"}`)
	h.Quiesce()

	// Close the tab: stop the CLI, keep the chat.
	resp := h.raw(http.MethodPost, wsBase(imported)+"/agent/chats/"+chatID+"/stop", nil, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	// The chat is DORMANT — its CLI was terminated and no runner points at it — but it
	// is STILL THERE (a 200, not the Delete route's 404), with its bound conversation
	// retained. That is exactly the state ResumeChat consumes.
	post := getAgentChat(t, h, wsBase(imported), chatID)
	assert.Empty(t, post.LiveRunnerID,
		"closing a chat tab must terminate its live CLI: the chat is dormant, with no runner to attach to")
	assert.Equal(t, []string{"sess-stop-1"}, post.sessionIDs(),
		"a close STOPS the process but must keep the conversation it can be resumed into — this is not a delete")

	// And it still appears in List — a close is not a delete.
	var list []agentChatDTO
	h.get(wsBase(imported)+"/agent/chats", &list)
	var found bool
	for _, c := range list {
		if c.ID == chatID {
			found = true
			assert.Empty(t, c.LiveRunnerID, "the listed chat is dormant after a stop")
		}
	}
	require.True(t, found, "a stopped chat must still appear in List — closing the tab does not delete it")
}
