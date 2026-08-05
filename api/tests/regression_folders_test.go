//go:build integration

package tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file pins the sidebar's organisation layer end to end: folders, the dense
// order folders and workspaces share, and the two guards that keep the tree
// renderable — no cycles, and no folder edge that splits a fork chain. Each test
// drives the real HTTP + WebSocket surface, so a refusal that only ever existed
// in the frontend would fail here.

// folderDTO mirrors the FolderDTO wire shape.
type folderDTO struct {
	ID        string `json:"id"`
	RepoID    string `json:"repoId"`
	ProjectID string `json:"projectId"`
	ParentID  string `json:"parentId"`
	Name      string `json:"name"`
	Order     int    `json:"order"`
	Status    string `json:"status"`
}

// repoBase returns the hierarchical repo route prefix for an imported repo.
func repoBase(
	imported importedRepo,
) string {
	return "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID
}

// createFolder posts a folder and returns the id the mutation envelope carries.
func createFolder(
	t *testing.T,
	h *harness,
	imported importedRepo,
	name string,
	parentID string,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/folders",
		map[string]string{"name": name, "parentId": parentID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "a folder create must answer with its id")
	return created.ID
}

func listFolders(
	t *testing.T,
	h *harness,
	imported importedRepo,
) []folderDTO {
	t.Helper()
	var folders []folderDTO
	h.get(repoBase(imported)+"/folders", &folders)
	return folders
}

// rootOrders returns the index every row at the repo ROOT holds — folders and
// fork-root workspaces alike, because the two kinds share one sibling space. The
// repo home is excluded: the frontend lifts it out of the tree and opens it from
// the repo header, so it holds no slot.
func rootOrders(
	t *testing.T,
	h *harness,
	imported importedRepo,
) map[string]int {
	t.Helper()
	h.Quiesce()
	out := map[string]int{}
	for _, f := range listFolders(t, h, imported) {
		if f.ParentID == "" {
			out[f.ID] = f.Order
		}
	}
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if w.ParentID == "" && w.FolderID == "" && !w.IsDefault && w.Status != "deleted" {
			out[w.ID] = w.Order
		}
	}
	return out
}

// assertDense pins the whole point of the index: every row at a level holds a
// distinct 0..n-1 slot, so the next drop index means what it says.
func assertDense(
	t *testing.T,
	orders map[string]int,
) {
	t.Helper()
	seen := make([]bool, len(orders))
	for id, order := range orders {
		require.GreaterOrEqual(t, order, 0, "row %s", id)
		require.Less(t, order, len(orders), "row %s: order %d is outside 0..%d", id, order, len(orders)-1)
		require.False(t, seen[order], "row %s: order %d is held twice", id, order)
		seen[order] = true
	}
}

// folderByID indexes a folder list so a test can assert about one row without
// depending on its position.
func folderByID(
	rows []folderDTO,
	id string,
) (folderDTO, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return folderDTO{}, false
}

// Folders are a plain GORM row with no aggregate projection to ride, so every
// mutation broadcasts itself. If it did not, a second window would keep showing
// a folder that was renamed or deleted in the first until it reconnected.
func TestRegression_FolderCRUDRidesTheFoldersStream(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := repoBase(imported)

	conn := h.dial(base + "/folders")

	folderID := createFolder(t, h, imported, "spikes", "")
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == folderID && m["name"] == "spikes"
	})
	assert.Equal(t, imported.projectID, created["projectId"])
	assert.Equal(t, imported.repoID, created["repoId"])

	rows := listFolders(t, h, imported)
	got, ok := folderByID(rows, folderID)
	require.True(t, ok, "the created folder must be listable")
	assert.Equal(t, "spikes", got.Name)

	// The sibling space folders share with workspaces already holds the repo's
	// adopted branch rows, so a new folder appends AFTER them and the level comes
	// out dense.
	orders := rootOrders(t, h, imported)
	assertDense(t, orders)
	assert.Equal(t, len(orders)-1, orders[folderID], "a new folder lands at the end of its level")

	h.patch(base+"/folders/"+folderID, map[string]string{"name": "experiments"}, nil)
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == folderID && m["name"] == "experiments"
	})

	h.del(base+"/folders/"+folderID, nil, http.StatusNoContent, nil)
	tombstone := readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == folderID && m["status"] == "deleted"
	})
	assert.Equal(t, "deleted", tombstone["status"],
		"the client cache drops the entity off the tombstone, not a refetch")
	assert.Empty(t, listFolders(t, h, imported), "the folder is gone from the list")
}

// A folder under a protected branch is the ORDINARY case, not an edge one: most
// of them will hang off `develop`. The locked row is a git fact about the
// branch; it says nothing about whether the sidebar may organise beneath it.
func TestRegression_FolderNestsUnderAProtectedBranch(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	var protected workspaceDTO
	h.get(wsBase(imported), &protected)
	require.Equal(t, "locked", protected.Status,
		"precondition: the adopted main worktree is a protected, locked row")

	folderID := createFolder(t, h, imported, "spikes", protected.ID)

	got, ok := folderByID(listFolders(t, h, imported), folderID)
	require.True(t, ok)
	assert.Equal(t, protected.ID, got.ParentID,
		"a folder may hang off a locked workspace")
}

// A move into a folder's own subtree would leave a set of rows unreachable from
// the repo root: they exist, nothing renders them, and nothing can drag them back
// out. Refused server-side, before any write.
func TestRegression_FolderMoveRefusedWhenItWouldCycle(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := repoBase(imported)

	outer := createFolder(t, h, imported, "outer", "")
	inner := createFolder(t, h, imported, "inner", outer)

	msg := h.mutationError(http.MethodPatch, base+"/folders/"+outer,
		map[string]string{"parentId": inner}, http.StatusConflict)
	assert.Contains(t, msg, "inside itself",
		"the refusal has to say what is wrong, not just refuse")

	h.mutationError(http.MethodPatch, base+"/folders/"+outer,
		map[string]string{"parentId": outer}, http.StatusConflict)

	// The refused moves changed nothing.
	rows := listFolders(t, h, imported)
	outerRow, ok := folderByID(rows, outer)
	require.True(t, ok)
	assert.Equal(t, "", outerRow.ParentID)
	innerRow, ok := folderByID(rows, inner)
	require.True(t, ok)
	assert.Equal(t, outer, innerRow.ParentID)
}

// The invariant the design names explicitly, and the reason folders got their
// own field rather than borrowing ParentID: a workspace with a fork parent
// renders under that parent, and three git paths (merge eligibility, the diff
// base, the reparent leaf check) resolve ParentID back to a workspace. Filing a
// forked child away from its parent is refused HERE, not merely avoided by the
// UI that happens to prevent it.
func TestRegression_WorkspaceMoveRefusedWhenItWouldSplitAForkChain(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := repoBase(imported)

	// A child forked off the locked default-branch workspace.
	conn := h.dial(base + "/workspaces")
	resp := h.raw(http.MethodPost, base+"/workspaces", map[string]string{
		"branch": "feature/forked", "parentId": imported.workspaceID,
	}, http.StatusAccepted)
	_ = resp.Body.Close()
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["branch"] == "feature/forked" && m["status"] == "new"
	})
	childID, _ := created["id"].(string)
	require.NotEmpty(t, childID)
	require.Equal(t, imported.workspaceID, created["parentId"],
		"precondition: the child carries a fork parent")

	folderID := createFolder(t, h, imported, "spikes", "")

	msg := h.mutationError(http.MethodPatch, base+"/workspaces/"+childID,
		map[string]string{"folderId": folderID}, http.StatusConflict)
	assert.Contains(t, msg, "fork parent")

	h.Quiesce()
	var child workspaceDTO
	h.get(base+"/workspaces/"+childID, &child)
	assert.Equal(t, imported.workspaceID, child.ParentID,
		"a refused placement must never rewrite the fork lineage")
}

// Sibling order is a dense index rebuilt on every move. Dense is what makes the
// next drop index mean what it says; stable is what stops a level re-shuffling
// under a repeated gesture.
func TestRegression_SidebarOrderIsDenseAndStableAfterEveryMove(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := repoBase(imported)

	a := createFolder(t, h, imported, "a", "")
	b := createFolder(t, h, imported, "b", "")
	c := createFolder(t, h, imported, "c", "")

	got := rootOrders(t, h, imported)
	assertDense(t, got)
	last := len(got) - 1
	require.Equal(t, last-2, got[a])
	require.Equal(t, last-1, got[b])
	require.Equal(t, last, got[c])

	// Drag c to the front, twice. The second move is the one that matters: an
	// index that shifted a slot per identical request would drift.
	for range 2 {
		h.patch(base+"/folders/"+c, map[string]int{"order": 0}, nil)
		got = rootOrders(t, h, imported)
		assertDense(t, got)
		assert.Equal(t, 0, got[c])
		assert.Less(t, got[a], got[b], "the rows c jumped keep their relative order")
	}

	// An index past the end clamps rather than failing: the client computed it
	// against a list that may already have moved.
	h.patch(base+"/folders/"+c, map[string]int{"order": 99}, nil)
	got = rootOrders(t, h, imported)
	assertDense(t, got)
	assert.Equal(t, len(got)-1, got[c], "an out-of-range index lands at the end")

	// Nesting one level down leaves BOTH levels dense — the one it left and the
	// one it joined.
	h.patch(base+"/folders/"+c, map[string]string{"parentId": a}, nil)
	got = rootOrders(t, h, imported)
	assertDense(t, got)
	_, stillAtRoot := got[c]
	assert.False(t, stillAtRoot, "the folder left the root level")

	nested, ok := folderByID(listFolders(t, h, imported), c)
	require.True(t, ok)
	assert.Equal(t, a, nested.ParentID)
	assert.Equal(t, 0, nested.Order, "the folder is the first row of its new level")
}

// A repo that changed projects while its workspaces did not would KEEP them and
// stop showing them: every hierarchical route and the WS namespace are keyed on
// the workspace's own projectId, so they would 404 under the new path and be
// unreachable under the old.
// The repo list is scoped by the project in its PATH, not by a query parameter.
//
// Reading the wrong one is invisible with a single project — an unfiltered list
// and a correctly filtered one are the same list — and catastrophic with two:
// every project renders every repo in the install, so the sidebar shows the same
// repo under each project and no tree beneath it belongs where it is drawn.
//
// This needs a SECOND project to fail, which is exactly why nothing caught it.
func TestRegression_RepoListIsScopedToTheProjectInThePath(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	otherPath := gitRepoWithCommit(t)
	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "second", "path": otherPath}, http.StatusAccepted)
	_ = resp.Body.Close()
	other := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == otherPath
	})
	otherID, _ := other["id"].(string)
	require.NotEmpty(t, otherID)
	require.NotEqual(t, imported.projectID, otherID)

	h.Quiesce()

	var mine []repoDTO
	h.get("/v0/projects/"+imported.projectID+"/repos", &mine)
	require.NotEmpty(t, mine, "precondition: the first project has a repo")
	for _, r := range mine {
		assert.Equal(t, imported.projectID, r.ProjectID,
			"repo %s belongs to %s but was listed under %s", r.ID, r.ProjectID, imported.projectID)
	}

	var theirs []repoDTO
	h.get("/v0/projects/"+otherID+"/repos", &theirs)
	for _, r := range theirs {
		assert.Equal(t, otherID, r.ProjectID,
			"repo %s belongs to %s but was listed under %s", r.ID, r.ProjectID, otherID)
	}

	// The decisive assertion: the first project's repo must not appear under the
	// second. Without the path-parameter fix both lists are the whole install.
	for _, r := range theirs {
		assert.NotEqual(t, imported.repoID, r.ID,
			"the other project listed a repo that is not its own")
	}
}

func TestRegression_RepoMovedBetweenProjectsKeepsItsWorkspaces(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	// A second project to move the repo into.
	otherPath := gitRepoWithCommit(t)
	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "other", "path": otherPath}, http.StatusAccepted)
	_ = resp.Body.Close()
	other := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == otherPath
	})
	otherID, _ := other["id"].(string)
	require.NotEmpty(t, otherID)

	// The read model is eventually consistent, and the move reads it to find the
	// workspaces it has to carry. Quiesce is the deterministic barrier.
	h.Quiesce()
	before := listWorkspaces(t, h, imported.projectID, imported.repoID)
	require.NotEmpty(t, before, "precondition: the repo has workspaces to carry")

	resp = h.raw(http.MethodPatch, repoBase(imported),
		map[string]string{"projectId": otherID}, http.StatusNoContent)
	_ = resp.Body.Close()
	h.Quiesce()

	moved := listWorkspaces(t, h, otherID, imported.repoID)
	require.Len(t, moved, len(before), "every workspace follows the repo")
	for _, ws := range moved {
		assert.Equal(t, otherID, ws.ProjectID)
	}

	var repos []repoDTO
	h.get("/v0/projects/"+otherID+"/repos", &repos)
	var found bool
	for _, repo := range repos {
		if repo.ID == imported.repoID {
			found = true
		}
	}
	assert.True(t, found, "the repo itself is listed under its new project")

	// And it is gone from the old one — the repo-scope guard 404s the stale path
	// rather than serving a repo from a project that no longer owns it.
	oldResp := h.raw(http.MethodGet, repoBase(imported), nil, http.StatusNotFound)
	_ = oldResp.Body.Close()
}

// Delete unfiles; it never destroys. A folder holds no worktrees, so removing
// the workspaces filed under it would take work the user only meant to move —
// and the children have to be BROADCAST too, or a second window keeps rendering
// them inside a folder that is gone.
func TestRegression_FolderDeleteReparentsChildrenRatherThanDeletingThem(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	outer := createFolder(t, h, imported, "outer", "")
	inner := createFolder(t, h, imported, "inner", outer)

	// File the writable workspace into the folder about to be deleted.
	h.Quiesce()
	h.patch(base+"/workspaces/"+imported.workspaceID, map[string]string{"folderId": outer}, nil)
	h.Quiesce()
	var filed workspaceDTO
	h.get(base+"/workspaces/"+imported.workspaceID, &filed)
	require.Equal(t, outer, filed.FolderID, "precondition: the workspace is in the folder")

	conn := h.dial(base + "/folders")
	h.del(base+"/folders/"+outer, nil, http.StatusNoContent, nil)

	// The reparented child is broadcast as a LIVE row, ahead of the tombstone. The
	// match names the post-delete state on purpose: the connection's
	// snapshot-on-subscribe already replayed this folder as it was, so matching
	// on the id alone would assert against the frame from before the delete.
	reparented := readUntil(t, conn, func(m map[string]any) bool {
		if m["id"] != inner {
			return false
		}
		_, nested := m["parentId"]
		return !nested
	})
	assert.Empty(t, reparented["status"], "a reparented row is live, not a tombstone")
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == outer && m["status"] == "deleted"
	})

	rows := listFolders(t, h, imported)
	_, gone := folderByID(rows, outer)
	assert.False(t, gone, "the folder itself is removed")
	survivor, ok := folderByID(rows, inner)
	require.True(t, ok, "a child folder is reparented, never deleted")
	assert.Equal(t, "", survivor.ParentID)

	h.Quiesce()
	var surfaced workspaceDTO
	h.get(base+"/workspaces/"+imported.workspaceID, &surfaced)
	assert.Equal(t, "", surfaced.FolderID,
		"a workspace filed in the folder surfaces at the repo root, it does not disappear")
	assert.Equal(t, imported.workspaceID, surfaced.ID)
}

// The "+" on a folder row has to answer two questions at once, and they have
// different answers: where the row is SHOWN, and what it is FORKED FROM. Sending
// the folder as the fork parent fails the create outright — a folder has no
// branch to resolve — so the folder travels as folderId while the fork base
// falls back to the repo's default branch.
func TestRegression_WorkspaceCreatedInAFolderIsFiledThereAndForksFromTheDefaultBranch(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := repoBase(imported)

	folderID := createFolder(t, h, imported, "spikes", "")

	conn := h.dial(base + "/workspaces")
	resp := h.raw(http.MethodPost, base+"/workspaces", map[string]string{
		"branch": "feature/in-a-folder", "folderId": folderID,
	}, http.StatusAccepted)
	_ = resp.Body.Close()
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["branch"] == "feature/in-a-folder"
	})
	wsID, _ := created["id"].(string)
	require.NotEmpty(t, wsID, "the create must produce a workspace")

	h.Quiesce()
	var got workspaceDTO
	h.get(base+"/workspaces/"+wsID, &got)
	assert.Equal(t, folderID, got.FolderID, "the row is filed under the folder it was created in")
	assert.Empty(t, got.ParentID, "placement is not lineage: the folder is never a fork parent")
	assert.Equal(t,
		strings.TrimSpace(runGitOut(t, imported.repoPath, "rev-parse", "main")),
		got.ForkPointSha,
		"with no fork parent the branch forks from the repo's default branch")

	// The tree renders a row by its folder, so a folderId nothing resolves would
	// file it where nothing draws it. Refused on the request path, like a
	// parentId that resolves to nothing.
	h.postError(base+"/workspaces",
		map[string]string{"branch": "feature/nowhere", "folderId": "no-such-folder"},
		http.StatusNotFound)
}

// A created row has to be GIVEN a slot, not left holding the zero value.
//
// Folders and fork-root workspaces share one sibling space, so a workspace that
// keeps Go's zero Order collides with whichever row already holds slot 0 — and
// since a create is the one moment the user is looking straight at the row, the
// new branch appears at the TOP of a level it should have joined at the end. It
// only shows up once something has been dragged (before that every row ties at
// zero and the arrival tiebreak happens to be right), which is what let it
// through: the level has to be reordered first for the collision to bite.
func TestRegression_CreatedWorkspaceTakesTheNextSlotRatherThanZero(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := repoBase(imported)

	// A level that has already been dragged, so its rows hold explicit indices.
	first := createFolder(t, h, imported, "first", "")
	second := createFolder(t, h, imported, "second", "")
	h.patch(base+"/folders/"+second, map[string]int{"order": 0}, nil)

	before := rootOrders(t, h, imported)
	assertDense(t, before)
	require.Equal(t, 0, before[second], "the drag put it at the front")
	require.Less(t, before[second], before[first])

	conn := h.dial(base + "/workspaces")
	resp := h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/appended"}, http.StatusAccepted)
	_ = resp.Body.Close()
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["branch"] == "feature/appended"
	})
	wsID, _ := created["id"].(string)
	require.NotEmpty(t, wsID, "the create must produce a workspace")

	h.Quiesce()
	after := rootOrders(t, h, imported)
	// Density is the assertion that fails today: the new row holds 0, which
	// `second` already holds.
	assertDense(t, after)
	assert.Equal(t, len(after)-1, after[wsID], "a created row joins its level at the end")
	assert.Equal(t, before[second], after[second], "the rows already there do not move")
	assert.Equal(t, before[first], after[first])
}

// The REST list and the WS snapshot are two answers to the same question. If
// they disagreed, the sidebar would reorder itself on every reconnect — which is
// why the sort lives in the one converter both go through.
func TestRegression_FolderSnapshotAgreesWithTheRESTList(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := repoBase(imported)

	a := createFolder(t, h, imported, "a", "")
	b := createFolder(t, h, imported, "b", "")
	c := createFolder(t, h, imported, "c", "")
	h.patch(base+"/folders/"+c, map[string]int{"order": 0}, nil)

	want := listFolders(t, h, imported)
	require.Len(t, want, 3)
	require.Equal(t, []string{c, a, b}, []string{want[0].ID, want[1].ID, want[2].ID})

	// A fresh subscriber's snapshot replays the same three rows in the same
	// order. Each frame's arrival is the signal; read exactly three.
	conn := h.dial(base + "/folders")
	got := make([]string, 0, 3)
	for range 3 {
		frame := readUntil(t, conn, func(m map[string]any) bool {
			_, ok := m["id"].(string)
			return ok
		})
		id, _ := frame["id"].(string)
		got = append(got, id)
	}
	assert.Equal(t, []string{c, a, b}, got,
		"the snapshot must replay the order the REST list serves")
}
