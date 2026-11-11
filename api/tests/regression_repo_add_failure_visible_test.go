//go:build integration

package tests

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// deleteKeyed sends DELETE path under Idempotency-Key key.
func deleteKeyed(t *testing.T, h *harness, path, key string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, h.url+path, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Idempotency-Key", key)
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, raw
}

// TestRegression_RepoAddFailureIsVisible: adding a repository that cannot be set up reports
// why; the client used to wait 30 s for a repo that had been rolled back.
func TestRegression_RepoAddFailureIsVisible(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	main, err := h.app.Repositories.Workspace.Get(t.Context(), f.mainWSID)
	require.NoError(t, err)
	slugDir := filepath.Dir(worktreepath.WorkspaceRoot(main.WorktreePath))

	status, raw := deleteKeyed(t, h, f.repoBase(), uuid.NewString())
	removed := awaitOperation(t, h, acceptedOperation(t, status, raw).ID, operationSettled)
	require.Equal(t, domain.OpSucceeded, removed.State, "%+v", removed.Error)
	h.QuiesceReactors()
	f.pushBranchToOrigin(t, "develop", "main", "develop\n")
	runGit(t, f.repoPath, "fetch", "origin")
	// The re-add has to create <slug> again for develop's worktree; a file there
	// is what it cannot get past. main stays the repo's own checkout.
	require.NoError(t, os.RemoveAll(slugDir))
	require.NoError(t, os.WriteFile(slugDir, []byte("in the way\n"), 0o600))

	reposPath := "/v0/projects/" + f.projectID + "/repos"
	status, raw = postKeyed(t, h, reposPath, uuid.NewString(), map[string]string{"name": "demo", "path": f.repoPath})
	added := acceptedOperation(t, status, raw)
	require.Equal(t, domain.OpRepoCreate, added.Kind)

	settled := awaitOperation(t, h, added.ID, operationSettled)
	require.Equal(t, domain.OpFailed, settled.State)
	require.NotNil(t, settled.Error)
	require.Equal(t, problem.Partial, settled.Error.Code)
	branch := itemLabelled(t, settled, "develop")
	require.Equal(t, domain.EntityWorkspace, branch.Ref.Kind)
	require.Equal(t, domain.OpFailed, branch.State)
	require.NotNil(t, branch.Error)
	require.Contains(t, strings.ToLower(branch.Error.Message), "not a directory")
	repoItem := itemLabelled(t, settled, "Add demo")
	require.Equal(t, domain.OpSucceeded, repoItem.State)

	h.Quiesce()
	var repos []map[string]any
	h.get(reposPath, &repos)
	require.Len(t, repos, 1, "the repo stays added")
	require.Equal(t, repoItem.Ref.ID, repos[0]["id"])
	home := false
	for _, c := range listChats(t, h, f.projectID, repoItem.Ref.ID) {
		home = home || (c.Worktree != nil && c.Worktree.IsDefault)
	}
	require.True(t, home, "with its repo-home workspace")
}

// TestRegression_AddingARepoWithOnlyMainSucceeds: a default protected name
// (main, develop, master) the repo does not have must not fail the add.
func TestRegression_AddingARepoWithOnlyMainSucceeds(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	status, raw := deleteKeyed(t, h, f.repoBase(), uuid.NewString())
	require.Equal(t, domain.OpSucceeded, awaitOperation(t, h, acceptedOperation(t, status, raw).ID, operationSettled).State)
	h.QuiesceReactors()

	status, raw = postKeyed(t, h, "/v0/projects/"+f.projectID+"/repos", uuid.NewString(),
		map[string]string{"name": "demo", "path": f.repoPath})
	settled := awaitOperation(t, h, acceptedOperation(t, status, raw).ID, operationSettled)

	require.Equal(t, domain.OpSucceeded, settled.State, "%+v", settled.Error)
	for _, item := range settled.Items {
		require.Equal(t, domain.OpSucceeded, item.State, "item %s: %+v", item.Label, item.Error)
		require.NotContains(t, []string{"develop", "master"}, item.Label)
	}
}
