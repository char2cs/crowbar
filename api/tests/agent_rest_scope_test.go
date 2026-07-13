//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProviderDescriptorYAML is a minimal valid agent provider descriptor
// (engine/agent/descriptor.go's Validate requires id, spawn.cmd,
// spawn.interactive_required, hooks.format, hooks.events.session_start.
// session_id, and hooks.events.turn_stop.message) that spawns the POSIX
// `true` utility instead of a real vendor CLI (claude/codex). `true` exits
// almost instantly with no I/O, so these tests never depend on claude/codex
// being installed and never leak a live PTY: SpawnChat's own onExit cleanup
// (agent.Usecase.onRunnerExit) reaps the runner's tmp dir shortly after
// Create returns, independent of the REST assertions below, which only read
// chat metadata rather than depending on the process staying alive.
//
// A chat spawned on it therefore goes DORMANT almost immediately (the PTY dies
// and the live-runner row goes with it — row-existence IS the liveness answer),
// so any test that needs a runner to still be PLACED while it fires hooks at it
// uses the `livestub` descriptor (`cat`, which holds its PTY open) instead.
const stubProviderDescriptorYAML = `id: stub
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`

// writeStubProviderDescriptor overrides the "stub" provider id at
// <home>/descriptors/stub.yaml — engine/agent.ResolveDescriptor's disk-override
// path, read against the SAME crowbarHome the harness's adapter.Container was
// opened with (adapter.WithHomeDir(h.home), mirrored by
// agentWorkspaceReader.WorktreeDir) — so agent-chat integration tests can spawn
// real AgentChats without depending on the real claude/codex CLIs.
func writeStubProviderDescriptor(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stub.yaml"), []byte(stubProviderDescriptorYAML), 0o644))
}

// agentChatDTO mirrors the wire shape of dto.AgentChatDTO.
//
// The chat carries no process state of its own: LiveRunnerID / TerminalSessionID /
// ActiveProviderID are DERIVED at read time from the runner projections and joined
// on by the handler. LiveRunnerID is the whole liveness contract — it names the
// vendor CLI placed on this chat, and "" is a MEANINGFUL value (the chat is
// DORMANT), never a missing one. There is deliberately no status field to mirror:
// a live-runner row exists exactly while its PTY does, so a second stored opinion
// about liveness could only drift from the process — the production bug the runner
// refactor deleted. (This mirror superseded the `activeSegmentId` + `segments[]`
// shape, which is gone from the wire along with AgentSegment itself.)
type agentChatDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`
	// LiveRunnerID is the runner placed on this chat, or "" when it is dormant.
	// It is the id every in-PTY hook callback carries (the crowbarSegmentID), so
	// it is also what these tests post as `segment_id`.
	LiveRunnerID string `json:"liveRunnerId"`
	// TerminalSessionID is that runner's PTY — the terminal session a chat pane
	// attaches to. Empty exactly when LiveRunnerID is.
	TerminalSessionID string `json:"terminalSessionId"`
	ActiveProviderID  string `json:"activeProviderId"`
}

// agentChatConversation mirrors one domain.ChatConversation on the wire: a
// conversation the chat has HOSTED. Append-only history, projected from runner
// events — it is what a segment really was, minus everything that described a
// process (no status, no PTY, no runner id), which is why it cannot drift.
type agentChatConversation struct {
	ChatID     string `json:"chatId"`
	ProviderID string `json:"providerId"`
	SessionID  string `json:"sessionId"`
}

// agentChatDetail mirrors dto.AgentChatDetailDTO: the chat plus the conversations
// it has hosted, oldest first (the append-only history that succeeded `segments`).
type agentChatDetail struct {
	agentChatDTO
	Conversations []agentChatConversation `json:"conversations"`
}

// getAgentChat reads GET <base>/agent/chats/:id. base is a workspace mount
// (wsBase) or a project-home mount, both of which serve the same shape.
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

// sessionIDs lists the conversation ids a chat has hosted, oldest first.
func (d agentChatDetail) sessionIDs() []string {
	out := make([]string, 0, len(d.Conversations))
	for _, c := range d.Conversations {
		out = append(out, c.SessionID)
	}
	return out
}

// createAgentChat creates a chat in imported's workspace via the nested
// .../workspaces/:wsId/agent/chats route using the stub provider, then
// quiesces the async read-model projection (harness.Quiesce, backed by
// app/repositories.Container.WaitQuiescent — asynx's WaitPublish, never a
// sleep/poll) so a subsequent plain REST List/Get against the store-backed
// projection is guaranteed to observe it. It returns the new chat's id.
func createAgentChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": "stub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	h.Quiesce()
	return created.ID
}

// TestAgentREST_Scope proves the workspace-scoped agent REST surface (Task 3):
// List returns only the subscribing workspace's own chats, GET-by-id 404s a
// chat anchored to a DIFFERENT workspace addressed through the wrong
// workspace's route, and Create anchors the new chat to the :wsId path param
// (not a body field).
func TestAgentREST_Scope(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)

	a := importWritableWorkspace(t, h)
	b := importWritableWorkspace(t, h)

	chatA := createAgentChat(t, h, a)
	chatB := createAgentChat(t, h, b)

	// List: workspace A's chat list must contain only its own chat, never B's.
	var listA []agentChatDTO
	h.get(wsBase(a)+"/agent/chats", &listA)
	require.Len(t, listA, 1, "workspace A's chat list must contain exactly its own chat")
	assert.Equal(t, chatA, listA[0].ID)
	assert.Equal(t, a.workspaceID, listA[0].WorkspaceID)

	// Get-by-id: addressing B's chat through A's workspace route must 404 —
	// indistinguishable from an unknown id, never leaking that the chat exists
	// in another workspace.
	resp := h.raw(http.MethodGet, wsBase(a)+"/agent/chats/"+chatB, nil, http.StatusNotFound)
	_ = resp.Body.Close()

	// Create: the chat created against A's route must be anchored to A's
	// workspace id (read from the :wsId path param, not a workspaceId body
	// field the caller could otherwise spoof).
	var gotA agentChatDTO
	h.get(wsBase(a)+"/agent/chats/"+chatA, &gotA)
	assert.Equal(t, a.workspaceID, gotA.WorkspaceID)

	// Sanity: B's own route still resolves its own chat (the 404 above is
	// scope-specific, not a general breakage).
	var gotB agentChatDTO
	h.get(wsBase(b)+"/agent/chats/"+chatB, &gotB)
	assert.Equal(t, b.workspaceID, gotB.WorkspaceID)
}
