//go:build integration

package tests

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deleteWorld is everything a chat delete may touch, read back through the
// public API and git: the repo's workspaces, its tree rows and the checkout.
type deleteWorld struct {
	Workspaces []string
	Chats      []string
	Head       string
	Branches   string
	Worktrees  string
}

func readDeleteWorld(t *testing.T, h *harness, f importFixture) deleteWorld {
	t.Helper()
	h.QuiesceReactors()
	h.Quiesce()
	var ws []workspaceDTO
	h.get(f.repoBase()+"/workspaces", &ws)
	var chats []agentChatDTO
	h.get(f.repoBase()+"/chats", &chats)
	w := deleteWorld{
		Head:      strings.TrimSpace(runGitOut(t, f.repoPath, "rev-parse", "--abbrev-ref", "HEAD")),
		Branches:  runGitOut(t, f.repoPath, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"),
		Worktrees: runGitOut(t, f.repoPath, "worktree", "list", "--porcelain"),
	}
	for _, x := range ws {
		w.Workspaces = append(w.Workspaces, strings.Join(
			[]string{x.ID, x.Branch, x.ParentID, x.Status, x.OwningChatID}, " "))
	}
	for _, c := range chats {
		w.Chats = append(w.Chats, strings.Join([]string{c.ID, c.Type, c.WorkspaceID, c.ParentID}, " "))
	}
	sort.Strings(w.Workspaces)
	sort.Strings(w.Chats)
	return w
}

// deleteChat sends DELETE for chatID and reports the status and body as-is.
func deleteChat(t *testing.T, h *harness, base, chatID string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, h.url+base+"/chats/"+chatID, nil)
	require.NoError(t, err)
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(raw)
}

// forkedMain is the field setup: the repo home holds a chat and a folder, and
// the locked managed main has fork children release/1.0 (locked) and feature-a,
// whose row is dragged out beside main when beside is set.
type forkedMain struct {
	importFixture
	mainOwner   string
	mainParent  string
	featureChat string
	featurePath string
}

func newForkedMain(t *testing.T, h *harness, beside bool) forkedMain {
	t.Helper()
	writePromoteStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)
	home := workspacesOnBranch(t, h, f.repoID, "")
	require.Len(t, home, 1)
	status, _, msg := postChat(t, h, f.repoBase(), map[string]any{"provider": "promotestub", "workspaceId": home[0].ID})
	require.Equalf(t, http.StatusCreated, status, "repo chat: %s", msg)
	h.post(f.repoBase()+"/chats/folders", map[string]any{"name": "Experiments", "parentId": ""}, http.StatusCreated, nil)
	f.pushBranchToOrigin(t, "release/1.0", "main", "release\n")
	f.pushBranchToOrigin(t, "feature-a", "main", "feature\n")
	status, raw := f.post(t, h, uuid.NewString(), map[string]any{"branches": []map[string]any{
		{"name": "release/1.0", "chatId": uuid.NewString(), "lock": true},
		{"name": "feature-a", "chatId": uuid.NewString(), "lock": false},
	}})
	awaitOperation(t, h, acceptedOperation(t, status, raw).ID, operationSettled)

	m := forkedMain{importFixture: f}
	main := workspacesOnBranch(t, h, f.repoID, "main")
	require.Len(t, main, 1)
	m.mainParent = main[0].ParentID
	for _, b := range []string{"release/1.0", "feature-a"} {
		child := workspacesOnBranch(t, h, f.repoID, b)
		require.Len(t, child, 1, b)
		require.Equal(t, main[0].ID, child[0].ParentID, "%s forks off main", b)
	}
	m.featurePath = workspacesOnBranch(t, h, f.repoID, "feature-a")[0].WorktreePath
	for _, c := range listChats(t, h, f.projectID, f.repoID) {
		switch {
		case c.Worktree == nil:
		case c.WorkspaceID == main[0].ID:
			m.mainOwner = c.ID
		case c.Worktree.Branch == "feature-a":
			m.featureChat = c.ID
		}
	}
	require.NotEmpty(t, m.mainOwner, "main has an owning chat")
	require.NotEmpty(t, m.featureChat, "feature-a has an owning chat")
	if beside {
		h.patch(f.repoBase()+"/chats/"+m.featureChat+"/placement", map[string]any{"parentId": "", "order": 9}, nil)
	}
	return m
}

func (m forkedMain) unlockMain(t *testing.T, h *harness) {
	t.Helper()
	h.raw(http.MethodPost, m.repoBase()+"/chats/"+m.mainOwner+"/lock",
		map[string]any{"locked": false}, http.StatusNoContent).Body.Close()
}

// Deleting main's owner answered 422 after its git cascade had already taken
// feature-a (whose row sat outside main's subtree), reminted release/1.0 under
// the gone main and re-attached the checkout. The delete must land whole.
func TestRegression_DeleteMainWithForkChildrenIsAllOrNothing(t *testing.T) {
	h := newHarness(t)
	m := newForkedMain(t, h, true)
	m.unlockMain(t, h)

	status, body := deleteChat(t, h, m.repoBase(), m.mainOwner)
	after := readDeleteWorld(t, h, m.importFixture)

	require.Equalf(t, http.StatusAccepted, status, "%s", body)
	requireNoDanglingLinks(t, after)
	release := workspacesOnBranch(t, h, m.repoID, "release/1.0")
	require.Len(t, release, 1, "the locked fork child survives")
	assert.Equal(t, m.mainParent, release[0].ParentID, "it hangs where main hung")
	assert.Empty(t, workspacesOnBranch(t, h, m.repoID, "feature-a"), "the unlocked fork child goes with main")
	for _, row := range after.Chats {
		assert.NotContains(t, row, m.featureChat, "feature-a's chat goes with its worktree")
	}
}

// requireNoDanglingLinks fails when a workspace or a tree row names a parent
// that no longer exists.
func requireNoDanglingLinks(t *testing.T, w deleteWorld) {
	t.Helper()
	ids := map[string]bool{"": true}
	for _, row := range append(append([]string{}, w.Workspaces...), w.Chats...) {
		ids[strings.Fields(row)[0]] = true
	}
	for _, row := range w.Workspaces {
		parts := strings.Split(row, " ")
		require.Truef(t, ids[parts[2]], "workspace names a parent that is gone: %s", row)
	}
	for _, row := range w.Chats {
		parts := strings.Split(row, " ")
		require.Truef(t, ids[parts[3]], "tree row names a parent that is gone: %s", row)
	}
}

// Whatever refuses a delete, it refuses before anything goes: the workspaces,
// the tree rows, their parent links, the branches and the checkout are as they were.
func TestRegression_ARefusedDeleteChangesNothing(t *testing.T) {
	locked := func(*testing.T, *harness, forkedMain) {}
	atRisk := func(t *testing.T, h *harness, m forkedMain) {
		m.unlockMain(t, h)
		require.NoError(t, os.WriteFile(filepath.Join(m.featurePath, "wip.txt"), []byte("wip\n"), 0o600))
	}
	cases := map[string]struct {
		beside  bool
		arrange func(t *testing.T, h *harness, m forkedMain)
	}{
		"main is still locked":                         {beside: false, arrange: locked},
		"main is still locked, feature-a beside it":    {beside: true, arrange: locked},
		"feature-a holds uncommitted work":             {beside: false, arrange: atRisk},
		"feature-a beside main holds uncommitted work": {beside: true, arrange: atRisk},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			m := newForkedMain(t, h, tc.beside)
			tc.arrange(t, h, m)
			before := readDeleteWorld(t, h, m.importFixture)

			status, body := deleteChat(t, h, m.repoBase(), m.mainOwner)

			require.Equalf(t, http.StatusConflict, status, "%s", body)
			require.Equal(t, before, readDeleteWorld(t, h, m.importFixture), "a refused delete changes nothing")
		})
	}
}
