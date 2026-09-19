//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Live-reported: "Can't create a folder inside a locked branch and then move
// [chats/workspaces] into it... the chat is now on the root of the repo,
// instead of being inside that folder." Investigation traced the live bug to
// a FRONTEND fork-lineage walk (drop-actions.ts's workspaceAnchor, fixed
// separately) that mistranslated a folder's owning-chat parent id and fired
// an unwanted reparent — but the user explicitly asked for durable
// INTEGRATION coverage of the scenario itself, not just a fix for wherever
// the bug happened to live. These two tests pin the backend leg end to end,
// over the real HTTP + event-store stack: a folder filed directly under a
// locked branch's own chat row, real or imported, stays exactly where it was
// filed once a second chat is moved into it — not promoted to the repo root,
// not left under the locked chat itself.

// The locked branch here is ALSO the repo's own default checkout —
// importProject's "main" fixture, detached-and-adopted as its own managed
// LOCKED worktree (see that fixture's own doc). This is the aliasLevel case
// that collapses a level's OWNER and its WORKSPACE id straight to the bare
// root ("") for SIBLING/order purposes — the one shape most likely to leak a
// "landed at root" bug if the tree snapshot's canonical-parent resolution
// ever conflated "this row's OWN level is the root" with "a child filed
// under this row is the root too".
func TestRegression_ChatMovedIntoFolderUnderLockedDefaultBranchStaysInFolder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)

	folder := createChatFolder(t, h, repoBase(imported), "docs", imported.chatID)
	require.Equal(t, imported.chatID, folder.ParentID,
		"the folder must be created as a child of the locked chat, not root-normalised")

	// The read path must agree with the create's own answer -- a folder whose
	// LIST disagrees with its CREATE is exactly the kind of drift that would
	// leave a later move planning against a stale container.
	listed := listChatFolders(t, h, repoBase(imported))
	got, ok := chatFolderByID(listed, folder.ID)
	require.True(t, ok, "the new folder must be listed back")
	require.Equal(t, imported.chatID, got.ParentID)

	c2 := createRootChatIn(t, h, imported, imported.workspaceID)

	placed := placeChat(t, h, repoBase(imported), c2, map[string]any{"parentId": folder.ID})
	require.Equal(t, folder.ID, placed.Chat.ParentID,
		"the chat must land inside the folder, not at the repo root or under the locked chat itself")
}

// The same move against a LOCKED branch that is NOT the repo's default
// checkout -- an ordinary fork, explicitly locked via the real /lock route,
// so its folder is filed under a genuine, addressable non-default anchor
// row rather than the repo header's own collapsed level.
func TestRegression_ChatMovedIntoFolderUnderLockedNonDefaultBranchStaysInFolder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h) // imported.workspaceID/chatID now name the unlocked "feature/write" child
	branchChatID := imported.chatID

	locked := true
	h.raw(http.MethodPost, repoBase(imported)+"/chats/"+branchChatID+"/lock",
		map[string]any{"locked": &locked}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	folder := createChatFolder(t, h, repoBase(imported), "docs", branchChatID)
	require.Equal(t, branchChatID, folder.ParentID,
		"the folder must be created as a child of the locked branch's own chat, not root-normalised")

	c2 := createRootChatIn(t, h, imported, imported.workspaceID)

	placed := placeChat(t, h, repoBase(imported), c2, map[string]any{"parentId": folder.ID})
	require.Equal(t, folder.ID, placed.Chat.ParentID,
		"the chat must land inside the folder, not at the repo root or under the locked branch itself")
}
