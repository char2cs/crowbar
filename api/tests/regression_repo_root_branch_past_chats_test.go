//go:build integration

package tests

import (
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repoRootRow struct {
	id    string
	kind  string
	order int
}

// repoRootLevel reads a repo's root level exactly as the sidebar draws it under
// the header row (rows-from-repo.ts): the default checkout's root chats — its
// OWNING chat excluded, that one IS the header — plus every locked branch row
// (drawn at its workspace's own placement), on one order.
func repoRootLevel(t *testing.T, h *harness, imported importedRepo, defaultWS string) []repoRootRow {
	t.Helper()
	var rows []repoRootRow
	var chats []struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		ParentID    string `json:"parentId"`
		Order       int    `json:"order"`
		Worktree    *struct {
			OwningChatID string `json:"owningChatId"`
		} `json:"worktree"`
	}
	h.get(repoBase(imported)+"/chats", &chats)
	for _, c := range chats {
		if c.ParentID != "" || c.WorkspaceID != defaultWS {
			continue
		}
		if c.Worktree != nil && c.Worktree.OwningChatID == c.ID {
			continue // the header row itself
		}
		rows = append(rows, repoRootRow{id: c.ID, kind: "chat", order: c.Order})
	}
	var workspaces []struct {
		ID        string `json:"id"`
		IsDefault bool   `json:"isDefault"`
		FolderID  string `json:"folderId"`
		Order     int    `json:"order"`
	}
	h.get(repoBase(imported)+"/workspaces", &workspaces)
	for _, w := range workspaces {
		if w.IsDefault || w.FolderID != "" {
			continue
		}
		rows = append(rows, repoRootRow{id: w.ID, kind: "branch", order: w.Order})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].order < rows[j].order })
	return rows
}

func defaultWorkspaceOf(t *testing.T, h *harness, imported importedRepo) string {
	t.Helper()
	var workspaces []struct {
		ID        string `json:"id"`
		IsDefault bool   `json:"isDefault"`
	}
	h.get(repoBase(imported)+"/workspaces", &workspaces)
	for _, w := range workspaces {
		if w.IsDefault {
			return w.ID
		}
	}
	t.Fatal("no default workspace")
	return ""
}

func createRootChatIn(t *testing.T, h *harness, imported importedRepo, wsID string) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]any{"provider": "promotestub", "workspaceId": wsID, "parentId": ""},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()
	h.Quiesce()
	return created.ID
}

func rootIDs(rows []repoRootRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.id)
	}
	return out
}

// A repo's root level is ONE list under its header row: the default
// checkout's chats and the repo's locked branches share it (rows-from-repo.ts;
// sidebar-drop-policy.ts lets a locked branch reorder past a chat sibling and
// a chat past a branch). PlaceWorkspace plans against a snapshot scoped to
// the BRANCH's own workspace (scopeForWorkspace -> foreignAtRoot), so the
// default checkout's root chats are foreign to it: the branch cannot be
// placed past them and their orders are never shifted.
func TestRegression_LockedBranchCannotReorderPastRepoRootChats(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	defaultWS := defaultWorkspaceOf(t, h, imported)
	require.NotEqual(t, defaultWS, imported.workspaceID)

	c1 := createRootChatIn(t, h, imported, defaultWS)
	c2 := createRootChatIn(t, h, imported, defaultWS)

	before := repoRootLevel(t, h, imported, defaultWS)
	t.Logf("before: %+v", before)
	require.Len(t, before, 3)
	require.Equal(t, imported.workspaceID, before[0].id, "the locked branch is drawn first")

	// Drag the locked branch to the END of the repo root (after c1 and c2):
	// the sidebar's index over the visible rows is 2.
	var placed struct {
		Chat struct {
			Order int `json:"order"`
		} `json:"chat"`
	}
	h.patch(repoBase(imported)+"/workspaces/"+imported.workspaceID+"/placement",
		map[string]any{"parentId": "", "order": 2}, &placed)
	h.Quiesce()

	after := repoRootLevel(t, h, imported, defaultWS)
	t.Logf("after branch->2: %+v (placed order %d)", after, placed.Chat.Order)
	assert.Equal(t, []string{c1, c2, imported.workspaceID}, rootIDs(after), "the branch must sort after both chats")
}

// The chat writer's view of the same level counts the default checkout's
// OWNING chat as a root sibling (withoutHomeOwner only drops a project-home
// owner), a row the sidebar never draws — so a chat dropped at the end of the
// visible level lands one slot early.
func TestRegression_RepoRootChatDropCountsTheInvisibleOwnerChat(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	defaultWS := defaultWorkspaceOf(t, h, imported)

	c1 := createRootChatIn(t, h, imported, defaultWS)
	c2 := createRootChatIn(t, h, imported, defaultWS)
	before := repoRootLevel(t, h, imported, defaultWS)
	t.Logf("before: %+v", before)
	require.Equal(t, []string{imported.workspaceID, c1, c2}, rootIDs(before))

	// Drag c1 AFTER c2 (the end of the level): rest = [branch, c2], index 2.
	h.patch(repoBase(imported)+"/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 2}, nil)
	h.Quiesce()

	after := repoRootLevel(t, h, imported, defaultWS)
	t.Logf("after c1->2: %+v", after)
	assert.Equal(t, []string{imported.workspaceID, c2, c1}, rootIDs(after), "c1 must land after c2")
}
