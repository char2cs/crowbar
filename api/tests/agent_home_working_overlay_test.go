//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// requireHomeRESTWorking asserts the PROJECT-HOME workspace's REST read
// (GET /v0/projects/:projectId/home) reports the working overlay as want. This is
// the home workspace's ONLY REST read path — it is served by the home endpoint,
// not by the repo-scoped workspaces list/detail — so it is the only place a
// project-home read can pick up the derived overlay.
//
// No polling and no timing: the agent-turn overlay is folded synchronously in the
// same axAgentChat projection callback that emits the turn_started/turn_stopped
// chat frames, so once the caller has observed that frame (and Quiesce has drained
// the event pipeline) the in-memory overlay is already settled and this plain GET
// is deterministic.
func requireHomeRESTWorking(
	t *testing.T,
	h *harness,
	projectID string,
	want bool,
) {
	t.Helper()
	var home workspaceDTO
	h.get("/v0/projects/"+projectID+"/home", &home)
	require.NotEmpty(t, home.ID, "GET /home must resolve the project-home workspace")
	require.Equalf(t, want, home.Working, "REST GET /home working for project %s", projectID)
}

// TestRegression_HomeWorkspaceWorkingReflectsAgentTurn proves the PROJECT-HOME
// workspace's REST read agrees with its live agent-turn state, the invariant the
// worktree-scoped TestRegression_WorkspaceWorkingReflectsAgentTurn pins for the
// repo-scoped List/Detail but cannot reach for the home: the home workspace is
// served by GET /v0/projects/:projectId/home, a DIFFERENT handler, which used to
// return the stored row untouched and therefore always reported working=false —
// even while an agent chat anchored to that home was mid-turn (observed live: the
// chat-row spinner lit, the home workspace's icon never did).
//
// A user_prompt hook (→ turn_started) must flip the home read to working=true and
// a turn_stop hook (→ turn_stopped) must flip it back, so the home workspace's
// icon keeps its spinner across a refetch exactly like a worktree's does. Both
// assertions block on real signals — the agent lifecycle WS frame plus Quiesce —
// never on time.
func TestRegression_HomeWorkspaceWorkingReflectsAgentTurn(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h) // cat stays alive so the segment is active for the turn
	projectID := createProjectForHome(t, h)
	homeBase := "/v0/projects/" + projectID + "/home"

	// Dial the HOME agent lifecycle WS before creating so no frame is missed; its
	// frames are the turn signals this test blocks on.
	frames := dialAgentWS(t, h, homeBase+"/agent/ws/chats")

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	waitForChatFrame(t, frames, created.ID, "created")
	h.Quiesce()

	detail := getAgentChat(t, h, homeBase, created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	// An idle chat leaves the home workspace idle.
	requireHomeRESTWorking(t, h, projectID, false)

	// user_prompt opens the turn: the home REST read must now report working.
	postHomeHook(t, h, homeBase, segID, "user_prompt", `{"prompt":"hi"}`)
	waitForChatFrame(t, frames, created.ID, "turn_started")
	h.Quiesce()
	requireHomeRESTWorking(t, h, projectID, true)

	// turn_stop closes it: the home REST read must flip back.
	postHomeHook(t, h, homeBase, segID, "turn_stop", `{"last_assistant_message":"done"}`)
	waitForChatFrame(t, frames, created.ID, "turn_stopped")
	h.Quiesce()
	requireHomeRESTWorking(t, h, projectID, false)
}
