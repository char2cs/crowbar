//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// liveStubProviderDescriptorYAML spawns `cat`, which stays alive on its PTY
// (no stdin EOF) so the segment remains active while user_prompt/turn_stop hooks
// fire — unlike the `true` stub, which exits instantly and ends its segment.
const liveStubProviderDescriptorYAML = `id: livestub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
`

func writeLiveStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "livestub.yaml"), []byte(liveStubProviderDescriptorYAML), 0o644))
}

// requireRESTWorking asserts that BOTH REST read paths — the repo-scoped List
// (GET .../workspaces) and the single-workspace Detail (GET .../workspaces/:wsId)
// — stamp the workspace's `working` overlay as want. It is the REST counterpart
// to the WS readUntil: the agent-turn overlay is folded synchronously in the SAME
// axAgentChat projection callback that re-broadcasts the workspace, so once the
// caller has observed the WS frame the in-memory overlay is already settled and
// these plain GETs need no polling (block on the WS signal, never on time).
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

	var detail workspaceDTO
	h.get(wsBase(imported), &detail)
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
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	h.Quiesce()

	detail := getAgentChat(t, h, wsBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	conn := h.dial(repoBase + "/workspaces")

	// user_prompt opens the turn.
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": "livestub", "event": "user_prompt",
		"payload_raw": `{"prompt":"hi"}`,
	}, http.StatusAccepted).Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["working"] == true
	})
	// REST reads (List + Detail) must now reflect the same live overlay.
	requireRESTWorking(t, h, imported, true)

	// turn_stop closes it.
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": "livestub", "event": "turn_stop",
		"payload_raw": `{"last_assistant_message":"done"}`,
	}, http.StatusAccepted).Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["working"] == false
	})
	// REST reads must have flipped back too.
	requireRESTWorking(t, h, imported, false)
}

// createLiveStubChat creates a chat on the livestub provider in ws and returns its
// chat id plus the id of the RUNNER placed on it — the crowbar segment id the
// in-PTY hook callbacks carry, and the id every `segment_id` hook field below is.
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
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	detail := getAgentChat(t, h, wsBase(imported), created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	return created.ID, detail.LiveRunnerID
}

// postAgentHook fires one in-PTY hook callback at the workspace's agent mount.
func postAgentHook(
	t *testing.T,
	h *harness,
	imported importedRepo,
	segID, event, payload string,
) {
	t.Helper()
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": "livestub", "event": event, "payload_raw": payload,
	}, http.StatusAccepted).Body.Close()
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
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	_, segA := createLiveStubChat(t, h, imported)
	_, segB := createLiveStubChat(t, h, imported)

	conn := h.dial(repoBase + "/workspaces")

	// Chat A opens a turn: idle → working. The ONE legitimate true frame.
	postAgentHook(t, h, imported, segA, "user_prompt", `{"prompt":"a"}`)
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["working"] == true
	})

	// Chat B opens a turn (workspace already working) and chat A closes its turn
	// (chat B still working). Neither changes the overlay's value. Quiesce blocks on
	// the event pipeline draining — a real signal, not a sleep — so the projection
	// has fully folded both events before the assertion below.
	postAgentHook(t, h, imported, segB, "user_prompt", `{"prompt":"b"}`)
	postAgentHook(t, h, imported, segA, "turn_stop", `{"last_assistant_message":"a done"}`)
	h.Quiesce()

	// The overlay must STILL be true: chat B is mid-turn. A workspace spinner that
	// dropped here would be the "one chat finished, both spinners cleared" bug.
	requireRESTWorking(t, h, imported, true)

	// Chat B closes the LAST turn: working → idle, the transition the FE needs.
	postAgentHook(t, h, imported, segB, "turn_stop", `{"last_assistant_message":"b done"}`)
	redundant := 0
	readUntil(t, conn, func(m map[string]any) bool {
		if m["id"] != imported.workspaceID {
			return false
		}
		if m["working"] == true {
			redundant++ // a rebroadcast that re-asserted a value nobody changed
			return false
		}
		return m["working"] == false
	})
	require.Zerof(t, redundant,
		"%d redundant workspace rebroadcast(s): each one costs a git merge-tree under the repo lock", redundant)

	requireRESTWorking(t, h, imported, false)
}
