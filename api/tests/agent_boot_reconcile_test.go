//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentChatDetail is the decoded GET .../agent/chats/:id wire shape (the fields
// the boot-reconcile assertions read).
type agentChatDetail struct {
	ID              string `json:"id"`
	ActiveSegmentID string `json:"activeSegmentId"`
	Segments        []struct {
		ID                string `json:"id"`
		CrowbarSegmentID  string `json:"crowbarSegmentId"`
		TerminalSessionID string `json:"terminalSessionId"`
		Status            string `json:"status"`
	} `json:"segments"`
}

func getAgentChat(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
) agentChatDetail {
	t.Helper()
	var detail agentChatDetail
	h.get(base+"/agent/chats/"+chatID, &detail)
	return detail
}

// TestRegression_AgentBootReconcile_DeadCLIProcess_EndsSegment is the live-bug
// guard for "the read model lies about a live agent after a daemon restart".
//
// Scenario (exactly the reproduction): a chat's vendor CLI runs in a PTY, its
// turn is open (Working), the daemon is restarted. The CLI process dies with the
// daemon — no event ever records that — so ReconcileOnBoot must end the chat's
// active segment and stop its turn on the next boot.
//
// The trap this pins: a graceful daemon stop SUSPENDS every live PTY (terminal
// Shutdown persists a "suspended" meta row) and the next boot's
// RestorePersistedSessions reloads it as a PTY-LESS PLACEHOLDER under the SAME
// session id. A liveness check that merely asks "is this id in the registry"
// therefore reads the dead CLI as alive and skips the reconcile — leaving the
// segment active forever and letting the agent pane re-attach to a placeholder
// that the terminal engine resurrects as a BARE SHELL.
//
// No timing: every step blocks on a real signal (WS frame / asynx Quiesce), and
// the restart is a real container teardown + rebuild over the same crowbar home.
func TestRegression_AgentBootReconcile_DeadCLIProcess_EndsSegment(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	writeLiveStubProviderDescriptor(t, h1) // `cat` — stays alive on its PTY like a real CLI
	imported := importProject(t, h1)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	frames := dialAgentWS(t, h1, homeBase+"/agent/ws/chats")

	var created struct {
		ID string `json:"id"`
	}
	h1.post(homeBase+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	waitForChatFrame(t, frames, created.ID, "created")

	h1.Quiesce()
	spawned := getAgentChat(t, h1, homeBase, created.ID)
	require.Len(t, spawned.Segments, 1)
	require.Equal(t, "active", spawned.Segments[0].Status)
	require.NotEmpty(t, spawned.ActiveSegmentID)
	segID := spawned.Segments[0].CrowbarSegmentID

	// Open a turn so the boot reconcile has a live turn to stop, not just a
	// segment to end (the crash-mid-turn shape the FE spinner depends on).
	postHomeHook(t, h1, homeBase, segID, "user_prompt", `{"prompt":"hi"}`)
	waitForChatFrame(t, frames, created.ID, "turn_started")
	h1.Quiesce()
	working, err := h1.app.Usecases.Agent.GetChat(context.Background(), created.ID)
	require.NoError(t, err)
	require.True(t, working.Working, "precondition: the chat is mid-turn before the restart")

	// --- daemon restart: graceful stop (suspends+persists live PTYs), then boot ---
	h1.shutdown()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	post := getAgentChat(t, h2, homeBase, created.ID)
	assert.Empty(t, post.ActiveSegmentID,
		"boot reconcile must clear ActiveSegmentID: the CLI process died with the daemon")
	require.Len(t, post.Segments, 1)
	assert.Equal(t, "ended", post.Segments[0].Status,
		"boot reconcile must END the segment whose CLI process is gone")

	reconciled, err := h2.app.Usecases.Agent.GetChat(context.Background(), created.ID)
	require.NoError(t, err)
	assert.False(t, reconciled.Working,
		"boot reconcile must stop the turn interrupted by the restart")
}
