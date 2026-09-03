//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegression_AgentHomeCallbacksReachDaemon proves the in-PTY CLI callbacks
// for a PROJECT-HOME workspace land on the home-group agent mount and are
// processed by the daemon. A project-home workspace resolves an EMPTY repo id, so
// scope.go builds its callback URLs as /v0/projects/:projectId/home/agent/...
// (the home branch), and home.Register serves them. This fires the hook + rename
// callbacks against that home path and asserts the agent lifecycle WS emits the
// resulting session_bound / turn_started / turn_stopped / title_set frames — the
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

	// Dial the HOME agent lifecycle WS BEFORE creating so no frame is missed. The
	// agentChatDef filter keys on the RequireHomeWorkspace-injected :wsId, so this
	// connection sees exactly the project-home workspace's chats.
	frames := dialAgentWS(t, h, homeBase+"/chats/ws")

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	waitForChatFrame(t, frames, created.ID, "created")

	h.Quiesce()
	// The runner placed on the chat: its id IS the crowbarSegmentID the in-PTY
	// `crowbar hook` callbacks carry, which is what these hooks post as segment_id.
	detail := getAgentChat(t, h, homeBase, created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	// session_start hook → the provider session binds (session_bound). Proves the
	// project-home /chats/hooks callback (repo=="" ⇒ home path) reaches the daemon.
	postHomeHook(t, h, homeBase, segID, "session_start", `{"session_id":"sess-home-1"}`)
	waitForChatFrame(t, frames, created.ID, "session_bound")

	// user_prompt hook → the turn opens (turn_started): the working overlay + chat
	// spinner signal, now proven for a PROJECT-HOME workspace.
	postHomeHook(t, h, homeBase, segID, "user_prompt", `{"prompt":"hi"}`)
	waitForChatFrame(t, frames, created.ID, "turn_started")

	// turn_stop hook → the turn closes (turn_stopped).
	postHomeHook(t, h, homeBase, segID, "turn_stop", `{"last_assistant_message":"done"}`)
	waitForChatFrame(t, frames, created.ID, "turn_stopped")

	// Agent rename callback (?source=agent) → title_set: the derived-title path,
	// which for project-home builds /home/chats/:id/rename via scope.go too.
	resp := h.raw(http.MethodPost, homeBase+"/chats/"+created.ID+"/rename?source=agent",
		map[string]string{"title": "Derived home title"}, http.StatusAccepted)
	_ = resp.Body.Close()
	waitForChatFrame(t, frames, created.ID, "title_set")
}

// postHomeHook forwards a raw hook payload to the project-home agent hooks
// endpoint exactly as the in-PTY `crowbar hook` callback does (body shape from
// cmd/crowbar/hook.go's runHook), and asserts the daemon accepts it (202). It is
// the HTTP-level stand-in for the vendor CLI's callback, whose URL scope.go now
// builds as the home path when the workspace has no repo id.
func postHomeHook(
	t *testing.T,
	h *harness,
	homeBase, segID, event, payloadRaw string,
) {
	t.Helper()
	resp := h.raw(http.MethodPost, homeBase+"/chats/hooks", map[string]string{
		"segment_id":  segID,
		"provider":    "livestub",
		"event":       event,
		"payload_raw": payloadRaw,
	}, http.StatusAccepted)
	_ = resp.Body.Close()
}
