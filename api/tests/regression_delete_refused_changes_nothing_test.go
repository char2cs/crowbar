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

	"github.com/char2cs/crowbar/api/internal/domain"
)

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

type forkedMain struct {
	importFixture
	mainOwner   string
	mainParent  string
	featureChat string
	featurePath string
}

func newForkedMain(t *testing.T, h *harness) forkedMain {
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
	return m
}

func (m forkedMain) reparentOntoRelease(t *testing.T, h *harness) (releaseOwner string) {
	t.Helper()
	release := workspacesOnBranch(t, h, m.repoID, "release/1.0")[0]
	for _, c := range listChats(t, h, m.projectID, m.repoID) {
		if c.Worktree != nil && c.WorkspaceID == release.ID {
			releaseOwner = c.ID
		}
	}
	require.NotEmpty(t, releaseOwner, "release/1.0 has an owning chat")
	require.NoError(t, h.raw(http.MethodPost, m.repoBase()+"/chats/"+releaseOwner+"/lock",
		map[string]any{"locked": false}, http.StatusNoContent).Body.Close())
	op := startOperation(t, h, m.repoBase()+"/chats/"+m.featureChat+"/reparent",
		map[string]any{"newParentId": release.ID})
	require.Equal(t, domain.OpSucceeded, awaitOperation(t, h, op.ID, operationSettled).State)
	h.Quiesce()
	return releaseOwner
}

func (m forkedMain) unlockMain(t *testing.T, h *harness) {
	t.Helper()
	require.NoError(t, h.raw(http.MethodPost, m.repoBase()+"/chats/"+m.mainOwner+"/lock",
		map[string]any{"locked": false}, http.StatusNoContent).Body.Close())
}

func TestRegression_DeleteMainWithForkChildrenIsAllOrNothing(t *testing.T) {
	h := newHarness(t)
	m := newForkedMain(t, h)
	m.unlockMain(t, h)

	status, body := deleteChat(t, h, m.repoBase(), m.mainOwner)
	after := readDeleteWorld(t, h, m.importFixture)

	require.Equalf(t, http.StatusOK, status, "%s", body)
	assert.JSONEq(t, `{"success":true,"data":{"takenChatIds":[]}}`, body,
		"feature-a's chat is in the selection, so nothing is taken beyond it")
	requireNoDanglingLinks(t, after)
	release := workspacesOnBranch(t, h, m.repoID, "release/1.0")
	require.Len(t, release, 1, "the locked fork child survives")
	assert.Equal(t, m.mainParent, release[0].ParentID, "it hangs where main hung")
	assert.Empty(t, workspacesOnBranch(t, h, m.repoID, "feature-a"), "the unlocked fork child goes with main")
	for _, row := range after.Chats {
		assert.NotContains(t, row, m.featureChat, "feature-a's chat goes with its worktree")
	}
}

func TestRegression_DeleteTakesAReparentedForkChildBeyondTheSelection(t *testing.T) {
	h := newHarness(t)
	m := newForkedMain(t, h)
	releaseOwner := m.reparentOntoRelease(t, h)

	status, body := deleteChat(t, h, m.repoBase(), releaseOwner)
	after := readDeleteWorld(t, h, m.importFixture)

	require.Equalf(t, http.StatusOK, status, "%s", body)
	assert.JSONEq(t, `{"success":true,"data":{"takenChatIds":["`+m.featureChat+`"]}}`, body,
		"the reply names the chat taken beyond the selection")
	requireNoDanglingLinks(t, after)
	assert.Empty(t, workspacesOnBranch(t, h, m.repoID, "release/1.0"))
	assert.Empty(t, workspacesOnBranch(t, h, m.repoID, "feature-a"), "the reparented child goes with release")
	assert.Len(t, workspacesOnBranch(t, h, m.repoID, "main"), 1, "main stays")
	for _, row := range after.Chats {
		assert.NotContains(t, row, m.featureChat)
	}
}

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

func TestRegression_ARefusedDeleteChangesNothing(t *testing.T) {
	locked := func(_ *testing.T, _ *harness, m forkedMain) string { return m.mainOwner }
	atRisk := func(t *testing.T, h *harness, m forkedMain) string {
		t.Helper()
		m.unlockMain(t, h)
		require.NoError(t, os.WriteFile(filepath.Join(m.featurePath, "wip.txt"), []byte("wip\n"), 0o600))
		return m.mainOwner
	}
	reparentedAtRisk := func(t *testing.T, h *harness, m forkedMain) string {
		t.Helper()
		owner := m.reparentOntoRelease(t, h)
		require.NoError(t, os.WriteFile(filepath.Join(m.featurePath, "wip.txt"), []byte("wip\n"), 0o600))
		return owner
	}
	cases := map[string]func(t *testing.T, h *harness, m forkedMain) string{
		"main is still locked":             locked,
		"feature-a holds uncommitted work": atRisk,
		"feature-a, reparented onto release outside its subtree, holds uncommitted work": reparentedAtRisk,
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			m := newForkedMain(t, h)
			target := arrange(t, h, m)
			before := readDeleteWorld(t, h, m.importFixture)

			status, body := deleteChat(t, h, m.repoBase(), target)

			require.Equalf(t, http.StatusConflict, status, "%s", body)
			require.Equal(t, before, readDeleteWorld(t, h, m.importFixture), "a refused delete changes nothing")
		})
	}
}
