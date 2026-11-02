//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentHomeCallbacksReachDaemon proves the in-PTY CLI callbacks
// for a PROJECT-HOME workspace land on the home-group agent mount and are
// processed by the daemon. A project-home workspace resolves an EMPTY repo id, so
// scope.go builds its callback URLs as /v0/projects/:projectId/home/agent/...
// (the home branch), and home.Register serves them. This fires the hook + rename
// callbacks against that home path and asserts the chat's projects-scope row
// shows the resulting conversation, turn and title — the
// live signals the FE working overlay, chat-row spinner, and derived titles rely
// on. Without BOTH the scope.go home branch (commit 2) AND the home mount
// (commit 1) these 404, so this is the guard for the spec's CRITICAL "chats work
// for ALL workspace kinds" (project-home) requirement that Task 5's worktree-only
// overlay test cannot catch.
func TestRegression_AgentHomeCallbacksReachDaemon(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h) // cat stays alive so the segment is active for the turn
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	feed := recordSidebar(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	feed.awaitListed(created.ID)

	h.Quiesce()
	// The runner placed on the chat: its id is what these hooks are posted as.
	detail := getAgentChat(t, h, homeBase, created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	// session_start hook → the chat binds the provider session. Proves the
	// project-home hook route (repo=="" ⇒ home path) reaches the daemon.
	postHook(t, h, homeBase, "livestub", segID, "session_start", `{"session_id":"sess-home-1"}`)
	feed.awaitConversation(created.ID, "sess-home-1")

	// user_prompt hook → the turn opens (turn_started): the working overlay + chat
	// spinner signal, now proven for a PROJECT-HOME workspace.
	postHook(t, h, homeBase, "livestub", segID, "user_prompt", `{"prompt":"hi"}`)
	feed.awaitTurnStart(created.ID)

	// turn_stop hook → the turn closes (turn_stopped).
	postHook(t, h, homeBase, "livestub", segID, "turn_stop", `{"last_assistant_message":"done"}`)
	feed.awaitTurnEnd(created.ID)

	// Agent rename callback (?source=agent) → title_set: the derived-title path,
	// which for project-home builds /home/chats/:id/rename via scope.go too.
	resp := h.raw(http.MethodPost, homeBase+"/chats/"+created.ID+"/rename?source=agent",
		map[string]string{"title": "Derived home title"}, http.StatusNoContent)
	_ = resp.Body.Close()
	feed.awaitChat(created.ID, "titled", func(chat map[string]any) bool { return chat["title"] == "Derived home title" })
}

// A hook whose runner's chat is gone is refused and dead-lettered on the home
// mount as on the repo one: no hook is acknowledged that nothing recorded.
func TestRegression_AHomeHookFromAnUnknownRunnerIsRefused(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"
	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	h.QuiesceReactors()
	l := stubLaunch(t, h, "livestub", getAgentChat(t, h, homeBase, created.ID).LiveRunnerID)
	require.NoError(t, h.raw(http.MethodDelete, homeBase+"/chats/"+created.ID, nil, http.StatusOK).Body.Close())
	h.QuiesceReactors()
	before := chatDeadLetters(t, h)

	resp := l.PostHook(t, h.url+homeBase, "livestub", "user_prompt", []byte(`{"prompt":"hi"}`)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, before+1, chatDeadLetters(t, h))
}
