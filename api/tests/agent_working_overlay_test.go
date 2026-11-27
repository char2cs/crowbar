//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

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
	startChat(t, h, repoBase, created.ID)

	detail := getAgentChat(t, h, repoBase, created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	feed := recordSidebar(t, h)

	postHook(t, h, repoBase, "livestub", segID, "user_prompt", `{"prompt":"hi"}`)
	awaitWorkspaceWorking(feed, imported.workspaceID, true)
	requireRESTWorking(t, h, imported, true)

	postHook(t, h, repoBase, "livestub", segID, "turn_stop", `{"last_assistant_message":"done"}`)
	awaitWorkspaceWorking(feed, imported.workspaceID, false)
	requireRESTWorking(t, h, imported, false)
}

func createLiveStubChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (chatID, runnerID string) {
	t.Helper()
	return createStubChat(t, h, imported, "livestub")
}

func TestRegression_WorkspaceWorkingOverlappingChatsBroadcastOnce(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	_, segA := createLiveStubChat(t, h, imported)
	_, segB := createLiveStubChat(t, h, imported)

	feed := recordSidebar(t, h)

	postHook(t, h, repoBase(imported), "livestub", segA, "user_prompt", `{"prompt":"a"}`)
	awaitWorkspaceWorking(feed, imported.workspaceID, true)

	postHook(t, h, repoBase(imported), "livestub", segB, "user_prompt", `{"prompt":"b"}`)
	postHook(t, h, repoBase(imported), "livestub", segA, "turn_stop", `{"last_assistant_message":"a done"}`)
	h.Quiesce()

	requireRESTWorking(t, h, imported, true)

	postHook(t, h, repoBase(imported), "livestub", segB, "turn_stop", `{"last_assistant_message":"b done"}`)
	idle := awaitWorkspaceWorking(feed, imported.workspaceID, false)
	redundant := -1
	for _, r := range feed.rows()[:idle] {
		if r.Kind == string(stream.KindWorkspace) && r.ID == imported.workspaceID && r.Data["working"] == true {
			redundant++
		}
	}
	require.Zerof(t, redundant,
		"%d redundant workspace rebroadcast(s): each one costs a git merge-tree under the repo lock", redundant)

	requireRESTWorking(t, h, imported, false)
}

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
