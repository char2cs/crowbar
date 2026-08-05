package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func patchPath() string { return "/v0/projects/p1/repos/r1/workspaces/w1" }

// The rename answers synchronously so the user learns the outcome while the
// inline editor is still open, rather than via a later LastError frame.
func TestPatch_Returns200AndPassesBranchThrough(t *testing.T) {
	hierarchy := &fakeHierarchy{renamed: domain.Workspace{ID: "w1", Branch: "feature/x"}}

	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPatch,
		patchPath(),
		`{"branch":"feature/x"}`,
	)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", hierarchy.gotRenameID)
	assert.Equal(t, "feature/x", hierarchy.gotRenameTo)
	assert.Contains(t, rec.Body.String(), "w1")
}

func TestPatch_MissingBranch_Returns400(t *testing.T) {
	hierarchy := &fakeHierarchy{}

	rec := do(
		newRouter(&fakeReader{}, hierarchy, &fakeRepos{}),
		http.MethodPatch,
		patchPath(),
		`{"branch":"   "}`,
	)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, hierarchy.gotRenameID, "a blank name must never reach the usecase")
}

func TestPatch_BadJSON_Returns400(t *testing.T) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPatch,
		patchPath(),
		`{`,
	)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// Every refusal the user can actually trigger has to arrive as a 409 with a
// readable reason, not a 500.
func TestPatch_RefusalsMapToConflict(t *testing.T) {
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
				patchPath(),
				`{"branch":"feature/x"}`,
			)

			assert.Equal(t, http.StatusConflict, rec.Code)
			assert.Contains(t, rec.Body.String(), refusal.Error())
		})
	}
}

// A drag is a placement, and it carries no branch — the old branch-is-required
// rule would have 400'd every one of them.
func TestPatch_PlacementNeedsNoBranch(t *testing.T) {
	placer := &fakePlacer{placed: domain.Workspace{ID: "w1", FolderID: "f1", Order: 2}}
	r, got, frames := newRouterWithPlacer(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}, placer)

	rec := do(r, http.MethodPatch, patchPath(), `{"folderId":"f1","order":2}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", got.gotWS)
	require.NotNil(t, got.gotIn.FolderID)
	assert.Equal(t, "f1", *got.gotIn.FolderID)
	require.NotNil(t, got.gotIn.Order)
	assert.Equal(t, 2, *got.gotIn.Order)
	assert.Empty(t, *frames, "no folder was renumbered, so nothing to fan out")
}

// A placement renumbers the level, and the folders at that level are a plain
// GORM row with no projection to ride — so this handler has to fan them out
// itself or their orders stay stale in every client cache.
func TestPatch_BroadcastsTheFoldersAPlacementRenumbered(t *testing.T) {
	placer := &fakePlacer{
		placed:  domain.Workspace{ID: "w1"},
		shifted: []domain.Folder{{ID: "f1", Order: 1}},
	}
	r, _, frames := newRouterWithPlacer(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}, placer)

	rec := do(r, http.MethodPatch, patchPath(), `{"order":0}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, *frames, 1)
	assert.Equal(t, "f1", (*frames)[0].ID)
	assert.Equal(t, 1, (*frames)[0].Order)
}

// An explicit empty folderId is a move back to the repo ROOT, and must be
// distinguishable from an absent one.
func TestPatch_EmptyFolderMeansTheRepoRoot(t *testing.T) {
	placer := &fakePlacer{placed: domain.Workspace{ID: "w1"}}
	r, got, _ := newRouterWithPlacer(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}, placer)

	rec := do(r, http.MethodPatch, patchPath(), `{"folderId":""}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, got.gotIn.FolderID)
	assert.Equal(t, "", *got.gotIn.FolderID)
}

// The invariant the spec names explicitly: a move that would separate a
// workspace from its fork parent is refused server-side, and reaches the user as
// a 409 rather than a 500.
func TestPatch_ForkChainSplitIsAConflict(t *testing.T) {
	placer := &fakePlacer{err: folder.ErrForkChainSplit}
	r, _, frames := newRouterWithPlacer(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}, placer)

	rec := do(r, http.MethodPatch, patchPath(), `{"folderId":"f1"}`)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Empty(t, *frames)
}

// A branch-only PATCH must not reach the placer at all, so a rename never
// renumbers a level as a side effect.
func TestPatch_BranchOnlySkipsThePlacer(t *testing.T) {
	placer := &fakePlacer{}
	hierarchy := &fakeHierarchy{renamed: domain.Workspace{ID: "w1", Branch: "feature/x"}}
	r, got, _ := newRouterWithPlacer(&fakeReader{}, hierarchy, &fakeRepos{}, placer)

	rec := do(r, http.MethodPatch, patchPath(), `{"branch":"feature/x"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, got.calls)
	assert.Equal(t, "feature/x", hierarchy.gotRenameTo)
}

// A failed rename must stop before the placement runs, or a refused PATCH would
// still half-apply.
func TestPatch_FailedRenameSkipsThePlacement(t *testing.T) {
	placer := &fakePlacer{}
	hierarchy := &fakeHierarchy{renameErr: worktree.ErrWorkspaceLocked}
	r, got, _ := newRouterWithPlacer(&fakeReader{}, hierarchy, &fakeRepos{}, placer)

	rec := do(r, http.MethodPatch, patchPath(), `{"branch":"feature/x","order":0}`)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Zero(t, got.calls)
}

// With no placer wired the endpoint answers 500 rather than panicking, matching
// the other degrade-rather-than-panic wirings.
func TestPatch_NoPlacer_500(t *testing.T) {
	rec := do(
		newRouter(&fakeReader{}, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPatch,
		patchPath(),
		`{"order":0}`,
	)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// An empty PATCH is a no-op that still answers, so a client that sent nothing
// gets a clean 200 rather than a spurious placement.
func TestPatch_EmptyBodyIsANoOp(t *testing.T) {
	placer := &fakePlacer{}
	hierarchy := &fakeHierarchy{}
	r, got, _ := newRouterWithPlacer(&fakeReader{}, hierarchy, &fakeRepos{}, placer)

	rec := do(r, http.MethodPatch, patchPath(), `{}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, got.calls)
	assert.Empty(t, hierarchy.gotRenameID)
}
