//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
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
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
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

// agentChatConversation mirrors one agents.ChatConversation on the wire: a
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

// getAgentChat reads GET <base>/chats/:id. base is a repo-scoped mount
// (repoBase) or a project-home mount, both of which serve the same shape.
func getAgentChat(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
) agentChatDetail {
	t.Helper()
	var detail agentChatDetail
	h.get(base+"/chats/"+chatID, &detail)
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

// createAgentChat creates a chat anchored to imported's workspace via the
// repo-scoped .../repos/:repoId/chats route (Task 17), naming the workspace
// explicitly in the body's workspaceId — the URL alone no longer anchors a
// create the way the retired .../workspaces/:wsId/chats route did — using the
// livestub provider, then quiesces the async read-model projection
// (harness.Quiesce, backed by app/repositories.Container.WaitQuiescent —
// asynx's WaitPublish, never a sleep/poll) so a subsequent plain REST
// List/Get against the store-backed projection is guaranteed to observe it.
// It returns the new chat's id.
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
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
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

// TestAgentREST_Scope proves the agent REST surface's isolation as it actually
// stands after Task 17's route rescope, replacing the workspace-scoped-route
// invariant this test pinned before that rescope deliberately relaxed it
// (model spec §5.1: "chats are addressed by id alone now").
//
// The HOME mount's isolation is UNCHANGED: RequireHomeWorkspace still injects
// the resolving project's own home :wsId, so requireChatInWorkspace still 404s
// a chat that belongs to a different project's home — proved here across TWO
// projects, each addressed only through its own home mount.
//
// The repo-scoped mount's WORKSPACE isolation is GONE by design: it has no
// :wsId path segment at all, so List spans every workspace in the repo and Get
// resolves any chat in it by id alone, regardless of which of the repo's OWN
// workspaces the chat actually lives in — proved here with two workspaces of
// one repo, each holding its own chat, both listed and both individually
// reachable through the repo's single mount.
//
// Its REPO isolation is not gone and never was: the list asserts, negatively,
// that neither of the other projects' home chats appears in it. That assertion
// is the one this test lacked while List fell back to the daemon-global chat
// list, which is how a repo-scoped read serving every repo's chats passed here
// unnoticed.
func TestAgentREST_Scope(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	// --- home mount: per-project isolation, unaffected by Task 17 ---
	projA := importProject(t, h)
	projB := importProject(t, h)
	homeA := "/v0/projects/" + projA.projectID + "/home"
	homeB := "/v0/projects/" + projB.projectID + "/home"

	var homeChatA struct {
		ID string `json:"id"`
	}
	h.post(homeA+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &homeChatA)
	require.NotEmpty(t, homeChatA.ID)
	h.QuiesceReactors()

	var homeChatB struct {
		ID string `json:"id"`
	}
	h.post(homeB+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &homeChatB)
	require.NotEmpty(t, homeChatB.ID)
	h.QuiesceReactors()

	var listA []agentChatDTO
	h.get(homeA+"/chats", &listA)
	require.Len(t, listA, 1, "project A's home chat list must contain exactly its own chat, never B's")
	assert.Equal(t, homeChatA.ID, listA[0].ID)

	homeResp := h.raw(http.MethodGet, homeA+"/chats/"+homeChatB.ID, nil, http.StatusNotFound)
	_ = homeResp.Body.Close()

	var gotHomeB agentChatDTO
	h.get(homeB+"/chats/"+homeChatB.ID, &gotHomeB)
	assert.Equal(t, homeChatB.ID, gotHomeB.ID, "sanity: B's own home mount still resolves its own chat")

	// --- repo-scoped mount: cross-workspace access is now legitimate ---
	imported := importProject(t, h)
	mainWS := imported.workspaceID
	otherWS := createChildWorkspace(t, h, repoBase(imported), "feature/other", mainWS)
	h.Quiesce()

	chatMain := createAgentChat(t, h, imported)
	other := imported
	other.workspaceID = otherWS
	chatOther := createAgentChat(t, h, other)

	base := repoBase(imported)
	var list []agentChatDTO
	h.get(base+"/chats", &list)
	ids := map[string]bool{}
	for _, c := range list {
		ids[c.ID] = true
	}
	assert.True(t, ids[chatMain], "the repo-scoped list carries the main workspace's own chat")
	assert.True(t, ids[chatOther], "and the repo's OTHER workspace's chat too — one list, whole repo")
	// The negative half, and the one this test was missing: cross-WORKSPACE
	// access inside a repo is legitimate, cross-REPO access never was. Both home
	// chats above live in projects with no repo at all, and asserting only what
	// the list DOES carry is exactly how the unfiltered fallback — every chat the
	// daemon knew, served into whichever repo asked — survived a green suite.
	assert.False(t, ids[homeChatA.ID],
		"another project's home chat must not appear in this repo's list")
	assert.False(t, ids[homeChatB.ID],
		"nor the second project's — a repo-scoped list is scoped to the repo")

	var gotMain agentChatDTO
	h.get(base+"/chats/"+chatMain, &gotMain)
	assert.Equal(t, mainWS, gotMain.WorkspaceID)

	var gotOther agentChatDTO
	h.get(base+"/chats/"+chatOther, &gotOther)
	assert.Equal(t, otherWS, gotOther.WorkspaceID,
		"a chat born in a DIFFERENT workspace of this repo is still reachable through the repo's own "+
			"mount: there is no :wsId segment left for the URL to get wrong")
}

// spawnStubChat creates a chat with the `true` provider WITHOUT asserting a status,
// because there are two honest ones and which arrives is a race (see below). It
// returns the status, the refusal message, and the created chat id — exactly one of
// the last two is ever non-empty.
func spawnStubChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (int, string, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"provider": "stub", "workspaceId": imported.workspaceID})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.url+repoBase(imported)+"/chats", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	if !env.Success {
		require.NotEmpty(t, env.Error, "error envelope must carry a message")
		return resp.StatusCode, env.Error, ""
	}
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &created))
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	return resp.StatusCode, "", created.ID
}

// TestRegression_ProviderThatExitsDuringStartupIsRefused pins the startup guard and
// the status it answers with.
//
// Crowbar hosts a provider's ordinary INTERACTIVE CLI in a real PTY — the one thing
// every descriptor must assert (spawn.interactive_required) and the engine refuses a
// descriptor that does not. A CLI that exits before its runner row can even be
// written has therefore not started, and a 201 there would hand back a chat whose
// pane attaches to a PTY that is already gone.
//
// The status is the other half. A vendor CLI that dies on startup — an expired
// login, a broken install, a CLI that refuses this workspace — is a DEPENDENCY
// failure the user can act on, not a daemon fault, so it answers 424 (the same class
// as the vendor CLI that is not installed at all) and says why. A 500 here is how
// this reaches a user as a button that silently does nothing.
//
// WHY THIS TEST BRANCHES. spawnRunner documents at length that exitedDuringStartup
// is narrower than its name: the PTY has to die BEFORE the runner row commits, which
// is a race between the OS reaping `true` and a sqlite write. The CLI has to LOSE it
// to be caught, and under a loaded machine it wins — the exit callback's goroutine
// is scheduled late, recordRunner commits first, and the honest answer is a 201 for a
// chat that reconciles into an ordinary dormant one. Asserting 424 flatly made this
// file fail roughly one full-suite run in two, on a DIFFERENT one of these two tests
// each time, while both passed 8/8 in isolation.
//
// So the invariant pinned here is the one that holds either way: THE RESPONSE AND THE
// RECORD NEVER CONTRADICT EACH OTHER. A refusal names the dependency and leaves
// nothing behind; a success hands back a chat that is honestly dormant. The
// deterministic half — that the guard fires at all, and discards — is driven exactly
// rather than hoped for in TestRegression_ProviderExitingBeforeItsRunnerRowCommits*,
// which forces the interleaving through the fake commander's fork seam.
func TestRegression_ProviderThatExitsDuringStartupIsRefused(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)

	status, msg, chatID := spawnStubChat(t, h, ws)
	h.QuiesceReactors()

	if status == http.StatusFailedDependency {
		assert.Contains(t, msg, "exited during startup",
			"the refusal has to name what went wrong: the user's own CLI died, and only they can fix it")
		return
	}

	require.Equal(t, http.StatusCreated, status,
		"a CLI that exits during startup is either refused as a dependency failure or "+
			"accepted as a chat that is already dormant — never any other status")
	var chat agentChatDTO
	h.get(repoBase(ws)+"/chats/"+chatID, &chat)
	assert.Empty(t, chat.LiveRunnerID,
		"the CLI is dead, so the chat it was handed back for must read as dormant")
	assert.Empty(t, chat.TerminalSessionID,
		"and must not point a pane at a PTY that is already gone")
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
// It branches for the same reason its sibling does, and the branch costs nothing:
// the chat COUNT is asserted against whichever answer the API actually gave, so a
// discard that stopped working fails the 424 branch and a create that stopped
// working fails the 201 branch. Neither branch can pass by accident, because
// `chats` is compared to an exact expected set, never merely inspected.
//
// The tail is what keeps it honest either way: the very same workspace still creates
// exactly ONE more chat with a provider that works.
func TestRegression_RefusedSpawnLeavesNoChatBehind(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)     // `true` — exits during startup
	writeLiveStubProviderDescriptor(t, h) // `cat` — holds its PTY open like a real CLI
	ws := importWritableWorkspace(t, h)

	status, _, refusedChatID := spawnStubChat(t, h, ws)
	h.QuiesceReactors()

	wantIDs := []string{}
	if status == http.StatusCreated {
		wantIDs = []string{refusedChatID}
	}

	require.Equal(t, wantIDs, chatIDs(t, h, repoBase(ws)),
		"the chat list must agree with the answer the API gave: a refusal leaves nothing behind")

	// Nor a directory of its own. Nothing under the workspace's chats dir may be
	// keyed by a chat that does not exist — only the two shared dirs the RUNNER and
	// hook lifecycles own, neither of which is a chat's.
	chatsDir, err := h.app.Usecases.AgentWorkspaceReader.AgentChatsDir(context.Background(), ws.workspaceID)
	require.NoError(t, err)
	entries, err := os.ReadDir(chatsDir)
	require.NoError(t, err)
	allowed := append([]string{"runners", ".hook-deliveries"}, wantIDs...)
	for _, entry := range entries {
		assert.Contains(t, allowed, entry.Name(),
			"a refused spawn left a per-chat directory behind")
	}

	created := createAgentChat(t, h, ws)
	require.Equal(t, append(wantIDs, created), chatIDs(t, h, repoBase(ws)),
		"the refusal cost the workspace nothing: a working provider still creates one chat")
}
