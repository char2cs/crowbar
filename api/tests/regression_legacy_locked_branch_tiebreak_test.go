//go:build integration

package tests

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

// drawnRow is one repo-root row with everything the sidebar's own comparator
// (row-order.ts compareSidebarRows: order, kind rank, createdAt, id) reads.
type drawnRow struct {
	id        string
	kind      string
	order     int
	createdAt string
}

func rankOfKind(kind string) int {
	switch kind {
	case "folder":
		return 0
	case "branch":
		return 1
	case "chat":
		return 2
	}
	return 3
}

// drawnRepoRoot reads a repo's root level and sorts it exactly as the sidebar
// draws it (rows-from-repo.ts: a locked branch row carries its WORKSPACE's
// createdAt, a chat row its own).
func drawnRepoRoot(t *testing.T, h *harness, imported importedRepo, defaultWS string) []drawnRow {
	t.Helper()
	var rows []drawnRow
	var chats []struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		ParentID    string `json:"parentId"`
		Order       int    `json:"order"`
		CreatedAt   string `json:"createdAt"`
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
			continue
		}
		rows = append(rows, drawnRow{id: c.ID, kind: "chat", order: c.Order, createdAt: c.CreatedAt})
	}
	var workspaces []struct {
		ID        string `json:"id"`
		IsDefault bool   `json:"isDefault"`
		FolderID  string `json:"folderId"`
		Order     int    `json:"order"`
		CreatedAt string `json:"createdAt"`
	}
	h.get(repoBase(imported)+"/workspaces", &workspaces)
	for _, w := range workspaces {
		if w.IsDefault || w.FolderID != "" {
			continue
		}
		rows = append(rows, drawnRow{id: w.ID, kind: "branch", order: w.Order, createdAt: w.CreatedAt})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.order != b.order {
			return a.order < b.order
		}
		if rankOfKind(a.kind) != rankOfKind(b.kind) {
			return rankOfKind(a.kind) < rankOfKind(b.kind)
		}
		if a.createdAt != b.createdAt {
			return a.createdAt < b.createdAt
		}
		return strings.Compare(a.id, b.id) < 0
	})
	return rows
}

func drawnIDs(rows []drawnRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.id)
	}
	return out
}

// legacyLockedBranch writes a locked branch the way every pre-Node production
// row exists: a workspace aggregate with no owning chat and no Node row. Its id
// and creation time are chosen by the caller so the two tiebreaks can be
// driven apart.
func legacyLockedBranch(t *testing.T, h *harness, imported importedRepo, id string, at time.Time) {
	t.Helper()
	_, err := h.app.Repositories.Workspace.Create(context.Background(), wsrepo.CreateInput{
		ID:           id,
		RepoID:       imported.repoID,
		ProjectID:    imported.projectID,
		Branch:       "release/" + id,
		WorktreePath: t.TempDir(),
		Protected:    true,
	}, at)
	require.NoError(t, err)
}

// Two protected branches imported before Node rows existed both carry order 0.
// The sidebar draws them by creation time (rows-from-repo.ts stamps a branch
// row with its workspace's createdAt); the daemon's repo-root densify never
// discovers a Node-less anchor at all (mergeForest walks Node.ListByParent
// only), and once the anchors do have Node rows it folds them in as
// workspaceAnchorView rows with a ZERO CreatedAt, so they tie by id instead.
// A chat dropped between the two branches therefore lands somewhere else.
func TestRegression_ChatDroppedBetweenLegacyLockedBranchesLandsWhereDrawn(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	defaultWS := defaultWorkspaceOf(t, h, imported)

	// Created FIRST, but with the lexically LATER id: the sidebar draws it
	// first, an id tiebreak draws it second.
	older := "zz-legacy-locked-older"
	newer := "aa-legacy-locked-newer"
	base := time.Now().Add(-2 * time.Hour)
	legacyLockedBranch(t, h, imported, older, base)
	legacyLockedBranch(t, h, imported, newer, base.Add(time.Hour))
	h.Quiesce()

	// The sidebar's boot reads (mint owners for the chatless rows).
	var seeded []struct {
		ID string `json:"id"`
	}
	h.get(repoBase(imported)+"/workspaces", &seeded)
	h.Quiesce()

	c1 := createRootChatIn(t, h, imported, defaultWS)

	before := drawnRepoRoot(t, h, imported, defaultWS)
	t.Logf("drawn before: %+v", before)
	require.Equal(t, []string{older, newer, imported.workspaceID, c1}, drawnIDs(before))

	// Drop c1 between older and newer: the drawn index over the rest is 1.
	h.patch(repoBase(imported)+"/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 1}, nil)
	h.Quiesce()

	after := drawnRepoRoot(t, h, imported, defaultWS)
	t.Logf("drawn after c1->1 (legacy, Node-less anchors): %+v", after)
	assert.Equal(t, []string{older, c1, newer, imported.workspaceID}, drawnIDs(after),
		"the chat must land between the two branches it was dropped between")
}

// The same drop once both legacy anchors HAVE Node rows (a thread was filed
// under each, which is what mints the row) — the tie is now between two
// discovered anchors, and the daemon breaks it by id because the anchor view
// carries no creation time.
func TestRegression_NodeBackedLockedBranchesTieByCreationNotID(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	defaultWS := defaultWorkspaceOf(t, h, imported)

	older := "zz-locked-older"
	newer := "aa-locked-newer"
	base := time.Now().Add(-2 * time.Hour)
	legacyLockedBranch(t, h, imported, older, base)
	legacyLockedBranch(t, h, imported, newer, base.Add(time.Hour))
	h.Quiesce()
	var seeded []struct {
		ID string `json:"id"`
	}
	h.get(repoBase(imported)+"/workspaces", &seeded)
	h.Quiesce()

	// Filing a thread under each branch mints its anchor Node at ("", 0).
	for _, ws := range []string{older, newer} {
		var created struct {
			ID string `json:"id"`
		}
		h.post(repoBase(imported)+"/chats",
			map[string]any{"provider": "promotestub", "parentId": ws, "workspaceId": ws},
			http.StatusCreated, &created)
		require.NotEmpty(t, created.ID)
	}
	h.QuiesceReactors()
	h.Quiesce()

	c1 := createRootChatIn(t, h, imported, defaultWS)
	before := drawnRepoRoot(t, h, imported, defaultWS)
	t.Logf("drawn before: %+v", before)
	require.Equal(t, []string{older, newer, imported.workspaceID, c1}, drawnIDs(before))

	h.patch(repoBase(imported)+"/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 1}, nil)
	h.Quiesce()

	after := drawnRepoRoot(t, h, imported, defaultWS)
	t.Logf("drawn after c1->1 (Node-backed anchors): %+v", after)
	assert.Equal(t, []string{older, c1, newer, imported.workspaceID}, drawnIDs(after),
		"the chat must land between the two branches it was dropped between")
}
