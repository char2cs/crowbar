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
// (agent.Usecase.onSegmentExit) reaps the segment's tmp dir shortly after
// Create returns, independent of the REST assertions below, which only read
// chat/segment metadata rather than depending on the process staying alive.
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

// agentChatDTO mirrors the wire shape of dto.AgentChatDTO (a strict subset:
// only the fields these tests assert on).
type agentChatDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
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
