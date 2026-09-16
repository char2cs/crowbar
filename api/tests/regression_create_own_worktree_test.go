//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createChatWithOwnWorktree posts { provider, parentId, ownWorktree: true } —
// deliberately with NO workspaceId in the body at all, since that is the one
// shape the atomic create actually applies to (Task 7's brief: "no workspaceId")
// — and returns the new chat's id.
func createChatWithOwnWorktree(
	t *testing.T,
	h *harness,
	base string,
	provider string,
	parentID string,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats",
		map[string]any{"provider": provider, "parentId": parentID, "ownWorktree": true},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	h.QuiesceReactors()
	return created.ID
}

// TestRegression_CreateChatWithOwnWorktreeIsAtomicOverHTTP proves Task 7's whole
// point: POST /repos/:rid/chats {parentId, providerId, ownWorktree:true} mints a
// REAL workspace (a real worktree on disk, cut from the correctly resolved fork
// parent — model spec §3.2's cwd-walk rule) AND the chat that owns it in ONE
// request, ONE response — never a chat-less workspace, nor a workspace-less
// chat waiting on a second call, in between.
//
// Before this task, POST /chats ignored an unknown "ownWorktree" field
// entirely and fell through to the plain workspace-less create: the new chat
// would come back as a bubble (WorkspaceID empty) with no worktree at all —
// exactly what every assertion below would catch.
//
// The fork parent is proven the same way TestRegression_PromoteFillsABubblesWorkspaceSlotOverHTTP
// proves Promote's: the new chat's CLI reports its own REAL OS-level working
// directory (readSpawnCwd), which is ground truth no Crowbar-reported field can
// fake. The new chat is threaded under a FOLDER filed under the ancestor chat,
// not directly under the ancestor: a chat parent still enforces the pre-existing
// same-workspace guard this task does not touch (see createOwnWorktreeChat's own
// doc comment, chat/internal/tree/chats.go) — the identical construction
// TestRegression_PromoteFillsABubblesWorkspaceSlotOverHTTP and
// TestRegression_BubbleChatSpawnsInAncestorWorktree already settled for.
func TestRegression_CreateChatWithOwnWorktreeIsAtomicOverHTTP(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	ancestorID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, "")
	ancestorCwd := readSpawnCwd(t, h, imported, ancestorID)
	require.NotEmpty(t, ancestorCwd, "the ancestor's own spawn must report a real cwd")

	folder := createChatFolder(t, h, base, "own-worktree-holder", ancestorID)

	newChatID := createChatWithOwnWorktree(t, h, base, "promotestub", folder.ID)

	detail := getAgentChat(t, h, base, newChatID)
	assert.Equal(t, folder.ID, detail.ParentID, "the create keeps the chat exactly where it was asked to be born")
	require.NotEmpty(t, detail.WorkspaceID, "ownWorktree must fill the workspace slot in the SAME call")
	assert.NotEqual(t, imported.workspaceID, detail.WorkspaceID,
		"the workspace must be a FRESH one forked from the resolved fork parent, not the ancestor's own")
	require.NotEmpty(t, detail.LiveRunnerID,
		"the atomic create must also start a live CLI, not merely mint a workspace")

	newCwd := readSpawnCwd(t, h, imported, newChatID)
	require.NotEmpty(t, newCwd, "the new chat's CLI must report a real cwd — a real worktree on disk")
	assert.NotEqual(t, ancestorCwd, newCwd,
		"the new chat's CLI must run in its OWN new worktree, not the ancestor's")
}

// TestRegression_CreateChatOwnWorktreeIsIgnoredWhenWorkspaceIdIsSupplied pins
// the single most important constraint on this task: a request that ALSO
// names an existing workspace takes the existing attach-to-existing-workspace
// path completely unchanged, whatever the body's ownWorktree says. Regressing
// this would mean every ordinary "new chat in this workspace" create starts
// forking a fresh worktree it was never asked for.
func TestRegression_CreateChatOwnWorktreeIsIgnoredWhenWorkspaceIdIsSupplied(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats",
		map[string]any{"provider": "promotestub", "workspaceId": imported.workspaceID, "ownWorktree": true},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()

	detail := getAgentChat(t, h, base, created.ID)
	assert.Equal(t, imported.workspaceID, detail.WorkspaceID,
		"a request naming an existing workspace must attach to it, not fork a new one")
}
