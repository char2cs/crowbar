//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These pin today's placement wire, so the Tree that replaces its storage has to
// answer every verb with the same parent and order, and refuse the same moves.

// placedRow is the placement half of every row the placement routes read back.
type placedRow struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	FolderID string `json:"folderId"`
	Order    int    `json:"order"`
}

// placementOf reads rowID's parent and order off the lists the sidebar reads:
// the repos (a repo's parent is its home folder), the project home's chats and
// folders, and each repo's chats and folders.
func placementOf(
	t *testing.T,
	h *harness,
	projectID string,
	rowID string,
) (parentID string, order int) {
	t.Helper()
	h.Quiesce()
	project := "/v0/projects/" + projectID
	lists := []string{project + "/home/chats", project + "/home/chats/folders"}
	var repos []placedRow
	h.get(project+"/repos", &repos)
	for _, r := range repos {
		if r.ID == rowID {
			return r.FolderID, r.Order
		}
		lists = append(lists, project+"/repos/"+r.ID+"/chats", project+"/repos/"+r.ID+"/chats/folders")
	}
	for _, list := range lists {
		var rows []placedRow
		h.get(list, &rows)
		for _, row := range rows {
			if row.ID == rowID {
				return row.ParentID, row.Order
			}
		}
	}
	require.Failf(t, "row missing", "no placement list holds %s", rowID)
	return "", 0
}

// placed is one row's expected parent and order.
type placed struct {
	id       string
	parentID string
	order    int
}

func requirePlaced(
	t *testing.T,
	h *harness,
	projectID string,
	verb string,
	want ...placed,
) {
	t.Helper()
	for _, w := range want {
		parentID, order := placementOf(t, h, projectID, w.id)
		got := placed{id: w.id, parentID: parentID, order: order}
		assert.Equal(t, w, got, "after %s", verb)
	}
}

// workspaceFolderOf reads a workspace's own folder edge off the workspace list.
func workspaceFolderOf(
	t *testing.T,
	h *harness,
	repoBase string,
	wsID string,
) string {
	t.Helper()
	var rows []placedRow
	h.get(repoBase+"/workspaces", &rows)
	for _, w := range rows {
		if w.ID == wsID {
			return w.FolderID
		}
	}
	require.Failf(t, "workspace missing", "GET /workspaces did not list %s", wsID)
	return ""
}

// importBranch imports branch from the fixture's origin and returns its workspace.
func importBranch(
	t *testing.T,
	h *harness,
	f importFixture,
	branch string,
) string {
	t.Helper()
	f.pushBranchToOrigin(t, branch, "main", branch+"\n")
	conn := h.dial(f.repoBase() + "/chats/ws")
	require.NoError(t, h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close())
	created := readUntil(t, conn, func(m map[string]any) bool { return m["kind"] == "workspace_set" })
	wsID, ok := created["workspaceId"].(string)
	require.True(t, ok && wsID != "", "import must broadcast a workspace for %s", branch)
	h.Quiesce()
	return wsID
}

// TestTree_PlacementVerbsOverHTTP: a "" parent is the root of the row's own
// scope (the project home, the repo, or a chat's workspace), and each verb
// renumbers the sibling space it lands in densely.
func TestTree_PlacementVerbsOverHTTP(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	p, home, repo := f.projectID, "/v0/projects/"+f.projectID+"/home", f.repoBase()

	ideas := createChatFolder(t, h, home, "Ideas", "").ID
	requirePlaced(t, h, p, "home folder create", placed{f.repoID, "", 0}, placed{ideas, "", 1})

	notes := createHomeChat(t, h, p)
	placeChat(t, h, home, notes, map[string]any{"parentId": ideas, "order": 0})
	requirePlaced(t, h, p, "home chat filed", placed{notes, ideas, 0}, placed{ideas, "", 1})

	experiments := createChatFolder(t, h, repo, "Experiments", "").ID
	requirePlaced(t, h, p, "repo folder create", placed{experiments, "", 1})

	ws := importBranch(t, h, f, "feature/a")
	owner := owningChatID(t, h, ws)
	requirePlaced(t, h, p, "branch import", placed{owner, f.mainWSID, 0})
	wip := createChatFolder(t, h, repo, "WIP", f.mainWSID).ID
	placeChat(t, h, repo, owner, map[string]any{"parentId": wip, "order": 0})
	requirePlaced(t, h, p, "branch filed under its fork parent", placed{owner, wip, 0}, placed{wip, f.mainWSID, 0})
	assert.Equal(t, wip, workspaceFolderOf(t, h, repo, ws), "the branch's workspace is filed too")
	assert.Equal(t, placementRefusal{http.StatusConflict, "tree_fork_chain"},
		patchRefusal(t, h, repo+"/chats/"+owner+"/placement", map[string]any{"parentId": experiments}),
		"a branch filed away from the branch it forks")

	thread := createChatWithProvider(t, h, repo, "livestub", ws, owner)
	requirePlaced(t, h, p, "thread create", placed{thread, owner, 0})

	placeChat(t, h, repo, thread, map[string]any{"parentId": "", "order": 0})
	requirePlaced(t, h, p, "thread to root", placed{thread, "", 0}, placed{experiments, "", 2})

	second := createHomeChat(t, h, p)
	requirePlaced(t, h, p, "second home chat",
		placed{f.repoID, "", 0}, placed{ideas, "", 1}, placed{second, "", 2})
	placeChat(t, h, home, second, map[string]any{"parentId": "", "order": 0})
	requirePlaced(t, h, p, "home root reorder",
		placed{second, "", 0}, placed{f.repoID, "", 1}, placed{ideas, "", 2})

	h.patch(repo+"/chats/folders/"+experiments, map[string]any{"name": "Spikes"}, nil)
	requirePlaced(t, h, p, "repo folder rename", placed{experiments, "", 2})

	h.del(repo+"/chats/folders/"+wip, nil, http.StatusOK, nil)
	requirePlaced(t, h, p, "folder delete", placed{owner, f.mainWSID, 0}, placed{thread, "", 0})
	assert.Equal(t, f.mainWSID, workspaceFolderOf(t, h, repo, ws), "the branch's workspace rises with its row")
}

// placementRefusal is the status and problem code a refused placement answers with.
type placementRefusal struct {
	status int
	code   string
}

// patchRefusal sends a placement PATCH that is expected to be refused.
func patchRefusal(
	t *testing.T,
	h *harness,
	path string,
	body any,
) placementRefusal {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, h.url+path, bytes.NewReader(encoded))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	var env struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	return placementRefusal{status: resp.StatusCode, code: env.Code}
}

// TestTree_PlacementRefusals: every refused move answers 409 with the code of
// the rule it breaks, and moves nothing.
func TestTree_PlacementRefusals(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	home, repo := "/v0/projects/"+f.projectID+"/home", f.repoBase()

	outer := createChatFolder(t, h, repo, "outer", "").ID
	inner := createChatFolder(t, h, repo, "inner", outer).ID
	homeFolder := createChatFolder(t, h, home, "Ideas", "").ID
	wsA := importBranch(t, h, f, "feature/a")
	wsB := importBranch(t, h, f, "feature/b")
	ownerB := owningChatID(t, h, wsB)
	chat := createChatWithProvider(t, h, repo, "livestub", f.mainWSID, "")
	rows := []string{outer, inner, homeFolder, chat, ownerB, owningChatID(t, h, wsA)}
	before := make([]placed, 0, len(rows))
	for _, id := range rows {
		parentID, order := placementOf(t, h, f.projectID, id)
		before = append(before, placed{id, parentID, order})
	}
	refused := func(code string) placementRefusal { return placementRefusal{http.StatusConflict, code} }

	assert.Equal(t, refused("tree_cycle"), patchRefusal(t, h, repo+"/chats/folders/"+outer,
		map[string]any{"parentId": inner}), "a folder moved into its own child")
	assert.Equal(t, refused("tree_cross_workspace"), patchRefusal(t, h, repo+"/chats/"+chat+"/placement",
		map[string]any{"parentId": ownerB}), "a chat moved under a chat of another workspace")
	assert.Equal(t, refused("tree_cross_context"), patchRefusal(t, h, repo+"/chats/folders/"+outer,
		map[string]any{"parentId": homeFolder}), "a repo folder moved to the home level")
	assert.Equal(t, refused("tree_fork_chain"), patchRefusal(t, h, repo+"/workspaces/"+wsA+"/placement",
		map[string]any{"parentId": ownerB}), "a forked workspace filed away from its fork parent")
	assert.Equal(t, refused("tree_not_a_container"), patchRefusal(t, h, home+"/chats/folders/"+homeFolder,
		map[string]any{"parentId": f.repoID}), "a home folder filed into a repo row")

	requirePlaced(t, h, f.projectID, "the refusals", before...)
}

// TestRegression_Tree_ABranchFiledInAFolderUnderAnotherBranchIsRefused: a
// folder hanging under one branch's row resolves to that branch, so filing
// another branch there splits its fork chain, through either placement route.
func TestRegression_Tree_ABranchFiledInAFolderUnderAnotherBranchIsRefused(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	repo := f.repoBase()
	wsA := importBranch(t, h, f, "feature/a")
	wsB := importBranch(t, h, f, "feature/b")
	ownerA, ownerB := owningChatID(t, h, wsA), owningChatID(t, h, wsB)
	underB := createChatFolder(t, h, repo, "under b", ownerB).ID
	parentID, order := placementOf(t, h, f.projectID, ownerA)
	folderID := workspaceFolderOf(t, h, repo, wsA)

	assert.Equal(t, placementRefusal{http.StatusConflict, "tree_fork_chain"},
		patchRefusal(t, h, repo+"/chats/"+ownerA+"/placement", map[string]any{"parentId": underB}))
	assert.Equal(t, placementRefusal{http.StatusConflict, "tree_fork_chain"},
		patchRefusal(t, h, repo+"/workspaces/"+wsA+"/placement", map[string]any{"parentId": underB}))

	requirePlaced(t, h, f.projectID, "the refusals", placed{ownerA, parentID, order})
	assert.Equal(t, folderID, workspaceFolderOf(t, h, repo, wsA))
}

// TestRegression_Tree_AChatWithNowhereToRunIsRefusedBeforeItIsMade: a repo
// chat create naming neither a workspace nor a parent row is a bad request,
// and leaves no chat behind.
func TestRegression_Tree_AChatWithNowhereToRunIsRefusedBeforeItIsMade(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	var before []placedRow
	h.get(f.repoBase()+"/chats", &before)

	resp := h.raw(http.MethodPost, f.repoBase()+"/chats", map[string]string{"provider": "livestub"},
		http.StatusBadRequest)
	var env struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "invalid_argument", env.Code)

	h.Quiesce()
	var after []placedRow
	h.get(f.repoBase()+"/chats", &after)
	assert.Equal(t, before, after, "no chat is left behind")
}

// TestTree_AMoveReachesALiveChatSocket: placement lives in the Tree, and a move
// still tells an open chats socket to re-read placement.
func TestTree_AMoveReachesALiveChatSocket(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	repo := f.repoBase()
	folder := createChatFolder(t, h, repo, "Experiments", "").ID
	chat := createChatWithProvider(t, h, repo, "livestub", f.mainWSID, "")
	h.Quiesce()
	conn := h.dial(repo + "/chats/ws")

	placeChat(t, h, repo, chat, map[string]any{"parentId": folder, "order": 0})

	readUntil(t, conn, func(m map[string]any) bool { return m["kind"] == "placement_set" })
	requirePlaced(t, h, f.projectID, "the move", placed{chat, folder, 0})
}

// TestTree_AThreadSpawnedAfterTheUpgradeInheritsItsParent: a thread of a
// thread is placed under the chat it was spawned from, read off the Tree.
func TestTree_AThreadSpawnedAfterTheUpgradeInheritsItsParent(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	p, repo := f.projectID, f.repoBase()
	ws := importBranch(t, h, f, "feature/a")
	owner := owningChatID(t, h, ws)

	thread := createChatWithProvider(t, h, repo, "livestub", ws, owner)
	nested := createChatWithProvider(t, h, repo, "livestub", ws, thread)

	requirePlaced(t, h, p, "threads spawned", placed{thread, owner, 0}, placed{nested, thread, 0})
}

// TestRegression_Tree_ARootForkMovesToItsRepoRootHoweverTheRootIsSpelled: main
// forks the repo's default checkout, so "", the default checkout's id and the
// repo row's id (its owning chat; the workspace route only, the chat route
// refuses a chat parent of another workspace) all name the level it belongs to.
func TestRegression_Tree_ARootForkMovesToItsRepoRootHoweverTheRootIsSpelled(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	repo := f.repoBase()
	home := workspacesOnBranch(t, h, f.repoID, "")
	require.Len(t, home, 1)
	homeOwner, mainOwner := owningChatID(t, h, home[0].ID), owningChatID(t, h, f.mainWSID)
	ok := placementRefusal{http.StatusOK, ""}

	for _, root := range []string{"", home[0].ID} {
		assert.Equal(t, ok, patchPlacement(t, h, repo+"/chats/"+mainOwner+"/placement", map[string]any{"parentId": root}),
			"chat route, root spelled %q", root)
	}
	for _, root := range []string{"", home[0].ID, homeOwner} {
		assert.Equal(t, ok, patchPlacement(t, h, repo+"/workspaces/"+f.mainWSID+"/placement", map[string]any{"parentId": root}),
			"workspace route, root spelled %q", root)
	}
}

// patchPlacement is patchRefusal for a PATCH that may succeed: a 200 reads as
// status 200 and no code.
func patchPlacement(t *testing.T, h *harness, path string, body any) placementRefusal {
	t.Helper()
	got := patchRefusal(t, h, path, body)
	if got.status == http.StatusOK {
		got.code = ""
	}
	return got
}
