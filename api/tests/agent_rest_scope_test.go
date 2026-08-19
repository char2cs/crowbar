//go:build integration

package tests

import (
	"context"
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
// `true` utility instead of a real vendor CLI (claude/codex).
//
// It is a descriptor that LIES, and that is now its whole job. Every descriptor
// must assert spawn.interactive_required — Crowbar hosts an interactive CLI in a
// real PTY and refuses a headless one (engine rule spawn_command) — and `true`
// exits before it has done anything at all. So it is the fixture for exactly one
// thing: proving a provider that exits during startup is REFUSED
// (TestRegression_ProviderThatExitsDuringStartupIsRefused), plus catalog-only
// tests that enumerate a provider without ever spawning it.
//
// Nothing that needs a chat to come up may use it. Those tests use `livestub`
// (`cat`, which holds its PTY open) — see createAgentChat. They once used this
// descriptor, back when a runner that died on the spot merely left the chat
// DORMANT; a chat whose CLI is already dead is now a failed create, not a chat.
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
	// ParentID is where the chat hangs in the Chats tree — another chat (making
	// this one a THREAD that reads its turns), a folder, or "" at the panel root —
	// and Order is its dense index within that parent's sibling space, which chats
	// share with chat folders.
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`
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
// .../workspaces/:wsId/agent/chats route using the livestub provider, then
// quiesces the async read-model projection (harness.Quiesce, backed by
// app/repositories.Container.WaitQuiescent — asynx's WaitPublish, never a
// sleep/poll) so a subsequent plain REST List/Get against the store-backed
// projection is guaranteed to observe it. It returns the new chat's id.
//
// livestub (`cat`) rather than stub (`true`) because a create only succeeds if
// the CLI is still there when its runner row commits: a provider that exits
// during startup is refused with 424, since the chat would otherwise open onto a
// PTY that is already gone. Callers must therefore write the LIVESTUB descriptor
// (writeLiveStubProviderDescriptor). None of them is testing the spawn — they
// need a chat to exist so they can exercise CRUD, scoping, deletion and the
// Chats tree over it — so the surviving fixture is the honest one.
func createAgentChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	// QuiesceReactors, not Quiesce: creating a chat also PLACES A RUNNER on it,
	// and that placement detaches into its own goroutine. A plain projection
	// drain sees the create handler "complete" the moment that goroutine is
	// spawned — so it can return while the chat still has no runner, which is
	// exactly when activeProviderId reads "". Joining the reactors is the real
	// signal that the spawned runner has actually landed.
	h.QuiesceReactors()
	return created.ID
}

// TestAgentREST_Scope proves the workspace-scoped agent REST surface (Task 3):
// List returns only the subscribing workspace's own chats, GET-by-id 404s a
// chat anchored to a DIFFERENT workspace addressed through the wrong
// workspace's route, and Create anchors the new chat to the :wsId path param
// (not a body field).
func TestAgentREST_Scope(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

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

// TestRegression_ProviderThatExitsDuringStartupIsRefused pins the startup guard and
// the status it answers with.
//
// Crowbar hosts a provider's ordinary INTERACTIVE CLI in a real PTY — the one thing
// every descriptor must assert (spawn.interactive_required) and the engine refuses a
// descriptor that does not. A CLI that exits before its runner row can even be
// written has therefore not started, and a 201 there would hand back a chat whose
// pane attaches to a PTY that is already gone: a live-looking chat with a corpse
// behind it, silent until the user typed into it.
//
// The status is the other half. A vendor CLI that dies on startup — an expired
// login, a broken install, a CLI that refuses this workspace — is a DEPENDENCY
// failure the user can act on, not a daemon fault, so it answers 424 (the same class
// as the vendor CLI that is not installed at all) and says why. A 500 here is how
// this reaches a user as a button that silently does nothing.
//
// The stub descriptor (`true`) exists for this test: a provider that swears it is
// interactive and exits immediately.
func TestRegression_ProviderThatExitsDuringStartupIsRefused(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)

	msg := h.mutationError(http.MethodPost, wsBase(ws)+"/agent/chats",
		map[string]string{"provider": "stub"}, http.StatusFailedDependency)

	assert.Contains(t, msg, "exited during startup",
		"the refusal has to name what went wrong: the user's own CLI died, and only they can fix it")
}

// TestRegression_RefusedSpawnLeavesNoChatBehind is the other half of the refusal
// above, and the half that was missing: the 424 was answered and the chat was
// created anyway.
//
// A 424 that also creates a chat is the record contradicting its own response. The
// API says the spawn failed; the state says a chat exists, and the user is left
// holding one they never made — with no CLI behind it and no idea where it came
// from. It is the same defect class as a prompt recorded `answered` whose bytes
// never reached the CLI, and a spinner still spinning after the CLI is done.
//
// The rule was already the codebase's own: TestRegression_DisabledProviderIsRefused-
// NotJustHidden asserts it in those words, but a disabled provider is refused BEFORE
// anything is written, so it could never have caught this. The chat is written
// mid-spawn (recordRunner), and only a refusal downstream of that point — a CLI that
// exits during startup — leaves one behind.
//
// The tail of the test is what keeps it honest: the very same workspace still creates
// exactly ONE chat with a provider that works, so the emptiness above is a discarded
// chat and not a create path this fixture had broken.
func TestRegression_RefusedSpawnLeavesNoChatBehind(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)     // `true` — exits during startup
	writeLiveStubProviderDescriptor(t, h) // `cat` — holds its PTY open like a real CLI
	ws := importWritableWorkspace(t, h)

	h.mutationError(http.MethodPost, wsBase(ws)+"/agent/chats",
		map[string]string{"provider": "stub"}, http.StatusFailedDependency)
	h.QuiesceReactors()

	var chats []agentChatDTO
	h.get(wsBase(ws)+"/agent/chats", &chats)
	require.Empty(t, chats, "a refused spawn must not leave a chat behind")

	// Nor a directory of its own. Nothing under the workspace's chats dir may be
	// keyed by a chat that does not exist — only the two shared dirs the RUNNER and
	// hook lifecycles own, neither of which is a chat's.
	chatsDir, err := h.app.Usecases.AgentWorkspaceReader.AgentChatsDir(context.Background(), ws.workspaceID)
	require.NoError(t, err)
	entries, err := os.ReadDir(chatsDir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.Contains(t, []string{"runners", ".hook-deliveries"}, entry.Name(),
			"a refused spawn left a per-chat directory behind")
	}

	created := createAgentChat(t, h, ws)
	h.get(wsBase(ws)+"/agent/chats", &chats)
	require.Len(t, chats, 1, "the refusal cost the workspace nothing: a working provider still creates one chat")
	assert.Equal(t, created, chats[0].ID)
}
