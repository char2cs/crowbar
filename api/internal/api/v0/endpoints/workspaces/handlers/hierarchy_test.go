package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMergeIntoParentSuccess(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeResult: worktree.MergeResult{ParentTipSha: "abc123"}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{"strategy":"squash"}`,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ConflictsPending bool   `json:"conflictsPending"`
			ParentTipSha     string `json:"parentTipSha"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.False(t, body.Data.ConflictsPending)
	assert.Equal(t, "abc123", body.Data.ParentTipSha)
	assert.Equal(t, "child", hierarchy.gotMergeID)
	assert.Equal(t, gitdomain.MergeStrategySquash, hierarchy.gotStrategy)
}

func TestMergeIntoParentConflicts(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeResult: worktree.MergeResult{ConflictsPending: true}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{"strategy":"merge"}`,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			ConflictsPending bool `json:"conflictsPending"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Data.ConflictsPending)
}

func TestMergeIntoParentBadJSON(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{not-json`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMergeIntoParentMissingStrategy(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMergeIntoParentLocked(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeErr: worktree.ErrParentLocked}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{"strategy":"merge"}`,
	)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestMergeIntoParentNonLeaf(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{mergeErr: worktree.ErrRebaseNonLeaf}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/merge-into-parent",
		`{"strategy":"rebase"}`,
	)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestReparentSuccess(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{reparented: domain.Workspace{ID: "child", ParentID: "np"}}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{"newParentId":"np"}`,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID       string `json:"id"`
			ParentID string `json:"parentId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "child", body.Data.ID)
	assert.Equal(t, "np", body.Data.ParentID)
	assert.Equal(t, "child", hierarchy.gotReparent)
	assert.Equal(t, "np", hierarchy.gotNewParent)
}

func TestReparentBadJSON(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{not-json`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReparentMissingNewParent(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReparentNonLeaf(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{reparentErr: worktree.ErrChildHasChildren}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{"newParentId":"np"}`,
	)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestReparentNewParentLocked(
	t *testing.T,
) {
	hierarchy := &fakeHierarchy{reparentErr: worktree.ErrNewParentLocked}
	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/reparent",
		`{"newParentId":"np"}`,
	)
	assert.Equal(t, http.StatusConflict, rec.Code)
}
