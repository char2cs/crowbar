//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// liveStubProviderDescriptorYAML runs the fake CLI, which stays alive on its PTY
// (no stdin EOF) so the segment remains active while user_prompt/turn_stop hooks
// fire — unlike the `true` stub, which exits instantly and ends its segment.
const liveStubProviderDescriptorYAML = `id: livestub
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

func writeLiveStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	writeProviderDescriptor(t, h, "livestub", liveStubProviderDescriptorYAML)
}

// requireRESTWorking asserts that BOTH REST read paths — the repo-scoped chat
// list (which carries every worktree-owning chat's git state inline) and one
// workspace's own row within it (worktreeOf; the dedicated GET
// .../workspaces/:wsId detail route is gone, spec §8 step 6) — stamp the
// workspace's `working` overlay as want. It is the REST counterpart to the
// workspace's projects-scope row: the overlay is folded synchronously in the SAME
// axAgentChat projection callback that republishes the workspace, so once the
// caller has observed that row these plain GETs need no polling.
func requireRESTWorking(
	t *testing.T,
	h *harness,
	imported importedRepo,
	want bool,
) {
	t.Helper()
	var found bool
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if w.ID == imported.workspaceID {
			found = true
			require.Equalf(t, want, w.Working, "REST List working for %s", imported.workspaceID)
		}
	}
	require.Truef(t, found, "workspace %s absent from REST List", imported.workspaceID)

	detail := worktreeOf(t, h, imported, imported.workspaceID)
	require.Equalf(t, want, detail.Working, "REST Detail working for %s", imported.workspaceID)
}

// TestRegression_WorkspaceWorkingReflectsAgentTurn proves the workspace `working`
// overlay (domain.Workspace.Working) is re-lit from live agent turns: a
// user_prompt hook (→ turn_started) re-broadcasts the workspace with working=true,
// and a turn_stop hook (→ turn_stopped) re-broadcasts it with working=false. It
// then re-reads BOTH REST paths (List + Detail) and requires they agree with the
// live frame — so a REST refetch mid-turn can never flicker the spinner off.
func TestRegression_WorkspaceWorkingReflectsAgentTurn(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase, created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	feed := recordSidebar(t, h)

	// user_prompt opens the turn.
	postHook(t, h, repoBase, "livestub", segID, "user_prompt", `{"prompt":"hi"}`)
	awaitWorkspaceWorking(feed, imported.workspaceID, true)
	// REST reads (List + Detail) must now reflect the same live overlay.
	requireRESTWorking(t, h, imported, true)

	// turn_stop closes it.
	postHook(t, h, repoBase, "livestub", segID, "turn_stop", `{"last_assistant_message":"done"}`)
	awaitWorkspaceWorking(feed, imported.workspaceID, false)
	// REST reads must have flipped back too.
	requireRESTWorking(t, h, imported, false)
}

// createLiveStubChat creates a chat on the livestub provider in ws and returns its
// chat id plus the id of the RUNNER placed on it — the crowbar segment id the
// CLI's hooks are posted as.
//
// It reads that id back as liveRunnerId (there is no segments[] any more: the chat
// aggregate stores no process facts at all, and the runner placed on it is joined on
// at read time off the runner projections). Quiesce is the read-your-writes barrier
// for that projection — asynx WaitPublish, never a sleep.
func createLiveStubChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (chatID, runnerID string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	return created.ID, detail.LiveRunnerID
}

// TestRegression_WorkspaceWorkingOverlappingChatsBroadcastOnce pins BOTH halves of
// the working overlay's rebroadcast rule for two chats working CONCURRENTLY in one
// workspace:
//
//   - Correctness: the overlay is the workspace's OR — it goes true on the first
//     turn, stays true while any chat is still mid-turn (so one chat finishing must
//     not clear the other's spinner), and only goes false on the last one.
//   - Cost: a rebroadcast is not free. broadcastWorkspace → enrichFrame →
//     eligibilityFor runs `git merge-tree --write-tree` under the per-clone git
//     mutex, so the projection may only rebroadcast when the overlay's VALUE
//     changes. The redundant events here (chat B's turn_started into an
//     already-working workspace; chat A's turn_stopped while B still works) change
//     nothing observable and must therefore emit no frame at all.
//
// The no-redundant-frame assertion needs no timing: WS frames are ordered, so any
// frame the redundant events would have produced necessarily arrives BEFORE the
// final working=false frame. Counting working=true frames for this workspace on the
// way to that false frame is an exact count of redundant rebroadcasts.
func TestRegression_WorkspaceWorkingOverlappingChatsBroadcastOnce(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	_, segA := createLiveStubChat(t, h, imported)
	_, segB := createLiveStubChat(t, h, imported)

	feed := recordSidebar(t, h)

	// Chat A opens a turn: idle → working. The ONE legitimate true frame.
	postHook(t, h, repoBase(imported), "livestub", segA, "user_prompt", `{"prompt":"a"}`)
	awaitWorkspaceWorking(feed, imported.workspaceID, true)

	// Chat B opens a turn (workspace already working) and chat A closes its turn
	// (chat B still working). Neither changes the overlay's value. Quiesce blocks on
	// the event pipeline draining — a real signal, not a sleep — so the projection
	// has fully folded both events before the assertion below.
	postHook(t, h, repoBase(imported), "livestub", segB, "user_prompt", `{"prompt":"b"}`)
	postHook(t, h, repoBase(imported), "livestub", segA, "turn_stop", `{"last_assistant_message":"a done"}`)
	h.Quiesce()

	// The overlay must STILL be true: chat B is mid-turn. A workspace spinner that
	// dropped here would be the "one chat finished, both spinners cleared" bug.
	requireRESTWorking(t, h, imported, true)

	// Chat B closes the LAST turn: working → idle, the transition the FE needs.
	postHook(t, h, repoBase(imported), "livestub", segB, "turn_stop", `{"last_assistant_message":"b done"}`)
	idle := awaitWorkspaceWorking(feed, imported.workspaceID, false)
	redundant := -1 // the first true row is chat A's legitimate one
	for _, r := range feed.rows()[:idle] {
		if r.Kind == string(stream.KindWorkspace) && r.ID == imported.workspaceID && r.Data["working"] == true {
			redundant++ // a rebroadcast that re-asserted a value nobody changed
		}
	}
	require.Zerof(t, redundant,
		"%d redundant workspace rebroadcast(s): each one costs a git merge-tree under the repo lock", redundant)

	requireRESTWorking(t, h, imported, false)
}

// awaitWorkspaceWorking waits for wsID's row whose working is want — an idle
// one only after a working one, since the snapshot's row is idle — and answers
// its index.
func awaitWorkspaceWorking(feed *sidebarFeed, wsID string, want bool) int {
	feed.t.Helper()
	isWS := func(r feedRow) bool { return r.Kind == string(stream.KindWorkspace) && r.ID == wsID && !r.Gone }
	at := -1
	feed.await(fmt.Sprintf("workspace %s working=%v", wsID, want), func(i int, r feedRow) bool {
		if !isWS(r) || r.Data["working"] != want {
			return false
		}
		if want {
			at = i
			return true
		}
		for _, before := range feed.rows()[:i] {
			if isWS(before) && before.Data["working"] == true {
				at = i
				return true
			}
		}
		return false
	})
	return at
}
