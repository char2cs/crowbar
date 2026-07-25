package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func renamePath() string { return "/v0/projects/p1/repos/r1/workspaces/w1" }

// The rename answers synchronously so the user learns the outcome while the
// inline editor is still open, rather than via a later LastError frame.
func TestRename_Returns200AndPassesBranchThrough(t *testing.T) {
	hierarchy := &fakeHierarchy{renamed: domain.Workspace{ID: "w1", Branch: "feature/x"}}

	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPatch,
		renamePath(),
		`{"branch":"feature/x"}`,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", hierarchy.gotRenameID)
	assert.Equal(t, "feature/x", hierarchy.gotRenameTo)
	assert.Contains(t, rec.Body.String(), "w1")
}

func TestRename_MissingBranch_Returns400(t *testing.T) {
	hierarchy := &fakeHierarchy{}

	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPatch,
		renamePath(),
		`{"branch":"   "}`,
	)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotRenameID, "a blank name must never reach the usecase")
}

func TestRename_BadJSON_Returns400(t *testing.T) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPatch,
		renamePath(),
		`{`,
	)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// Every refusal the user can actually trigger has to arrive as a 409 with a
// readable reason, not a 500.
func TestRename_RefusalsMapToConflict(t *testing.T) {
	for name, refusal := range map[string]error{
		"locked workspace":   worktree.ErrWorkspaceLocked,
		"branch taken":       worktree.ErrBranchWorkspaceExists,
		"destination taken":  worktree.ErrRenameTargetExists,
		"adopted checkout":   worktree.ErrRenameUnmanagedWorkspace,
		"not yet provisoned": worktree.ErrParentUnprovisioned,
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(
				newRouter(&fakeReader{}, &fakeHierarchy{renameErr: refusal}, &fakeRepos{}),
				http.MethodPatch,
				renamePath(),
				`{"branch":"feature/x"}`,
			)

			assert.Equal(t, http.StatusConflict, rec.Code)
			assert.Contains(t, rec.Body.String(), refusal.Error())
		})
	}
}
