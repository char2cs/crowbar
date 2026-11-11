package hierarchy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// errBoom2 is a second, distinct sentinel used where a test must prove that
// one failure (e.g. a best-effort cleanup) does NOT mask another (the real
// error the caller needs to see).
var errBoom2 = errors.New("boom2")

func placeholderWS(id, repoID, projectID, branch string) *fakeWorkspace {
	return &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: repoID, ProjectID: projectID, Branch: branch,
				Status: domain.WorkspaceStatusLocked,
			}, nil
		},
	}
}

// --- RetryProvision: top-level error propagation ---

func TestRetryProvision_GetWorkspaceError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

func TestRetryProvision_RepoPathError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

// TestRetryProvision_RepoRowMissing proves repoPathFor's repo-not-found branch
// (distinct from a lookup ERROR) surfaces as a plain error rather than
// dereferencing a nil repo.
func TestRetryProvision_RepoRowMissing(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRetryProvision_CrowbarHomeError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	badHome := func() (string, error) { return "", errBoom }
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), badHome)
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

func TestRetryProvision_HolderResolveError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	g := &fakeGit{worktreeListErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

// TestRetryProvision_DeriveWorktreePathError proves a path-derivation failure
// (here: an empty ProjectID, which worktreepath.Derive rejects) is surfaced
// as problem.ErrInvalidArgument rather than reaching git at all.
func TestRetryProvision_DeriveWorktreePathError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "", "develop")
	g := &fakeGit{} // Free: no holder
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.Error(t, err)
	assert.NotContains(t, g.ops(), "WorktreeAdd", "no worktree materialization when the path cannot be derived")
}

func TestRetryProvision_MaterializeError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	g := &fakeGit{worktreeAddErr: errBoom} // not on origin (default) -> plain WorktreeAdd
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, g.ops(), "WorktreeAdd")
}

func TestRetryProvision_ProvisionInPlaceError_CleansUpWorktree(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ProvisionInPlaceFn = func(_, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, g.ops(), "WorktreeRemove", "the orphaned worktree must be cleaned up when the row never lands")
}

// TestRetryProvision_ProvisionInPlaceError_CleanupAlsoFails proves the ORIGINAL
// persist error is still what's returned even when the best-effort worktree
// cleanup itself fails — the cleanup failure must never mask the real error.
func TestRetryProvision_ProvisionInPlaceError_CleanupAlsoFails(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ProvisionInPlaceFn = func(_, _, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{removeErr: errBoom2}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom, "the persist error must surface, not the cleanup error")
	assert.NotErrorIs(t, err, errBoom2)
}

// TestRetryProvision_NotOnOrigin_RevParseFails_ForkPointNonEssential proves that
// when a branch is materialised from the LOCAL ref (not on origin) and the
// follow-up RevParse to record its tip fails, provisioning still succeeds with
// an empty fork point — the worktree itself is already valid, and the fork
// point is a nice-to-have, not a correctness requirement here.
func TestRetryProvision_NotOnOrigin_RevParseFails_ForkPointNonEssential(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	var gotSha string
	provisioned := false
	ws.ProvisionInPlaceFn = func(_, _, sha string) (domain.Workspace, error) {
		gotSha = sha
		provisioned = true
		return domain.Workspace{}, nil
	}
	g := &fakeGit{revParseErr: errBoom} // not on origin (default); WorktreeAdd succeeds
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err)
	assert.True(t, provisioned)
	assert.Empty(t, gotSha, "an unresolvable fork point must not fail provisioning")
}

// TestRetryProvision_OriginBranch_FetchRefFails_StillMaterializesFromLocalRef
// proves originHasBranch's own best-effort contract: when the local
// remote-tracking ref origin/<branch> is present but refreshing it fails
// (offline), the branch is still classified as "on origin" and checked out
// from whatever the LOCAL remote-tracking ref already holds.
func TestRetryProvision_OriginBranch_FetchRefFails_StillMaterializesFromLocalRef(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	var gotSha string
	ws.ProvisionInPlaceFn = func(_, _, sha string) (domain.Workspace, error) {
		gotSha = sha
		return domain.Workspace{}, nil
	}
	g := &fakeGit{trackingExists: true, fetchRefErr: errBoom, revParseSha: "localoriginsha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err, "a failed refresh of origin/<branch> must not block provisioning from the local ref")
	assert.Equal(t, "localoriginsha", gotSha)
	assert.Contains(t, g.ops(), "WorktreeAddAtRef", "must still check out via the origin-ref path")
}

// TestRetryProvision_OriginBranch_MaterializesFromOriginAndTracks proves that
// when the branch is resolvable via the local remote-tracking ref,
// RetryProvision checks it out AT origin's ref (not the local branch) and
// links it back to origin via SetUpstream.
func TestRetryProvision_OriginBranch_MaterializesFromOriginAndTracks(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	var gotID, gotPath, gotSha string
	ws.ProvisionInPlaceFn = func(id, path, sha string) (domain.Workspace, error) {
		gotID, gotPath, gotSha = id, path, sha
		return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Provisioning: domain.WorkspaceProvisioned}, nil
	}
	g := &fakeGit{trackingExists: true, revParseSha: "originsha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err)
	assert.Equal(t, "ph", gotID)
	assert.NotEmpty(t, gotPath)
	assert.Equal(t, "originsha", gotSha)
	assert.Contains(t, g.ops(), "WorktreeAddAtRef", "must check out AT origin's ref")
	assert.NotContains(t, g.ops(), "WorktreeAdd", "must not use the plain (local-ref) checkout")
	assert.Contains(t, g.ops(), "SetUpstream", "must link the checkout back to origin")
}

func TestRetryProvision_OriginBranch_WorktreeAddAtRefError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	g := &fakeGit{trackingExists: true, worktreeAddErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
	assert.NotContains(t, g.ops(), "SetUpstream", "no upstream link attempted when the checkout itself failed")
}

// TestRetryProvision_OriginBranch_SetUpstreamError_IsBestEffort proves a
// SetUpstream failure never fails provisioning: the checkout content is
// already correct, only ahead/behind reporting may be degraded.
func TestRetryProvision_OriginBranch_SetUpstreamError_IsBestEffort(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	var gotSha string
	ws.ProvisionInPlaceFn = func(_, _, sha string) (domain.Workspace, error) {
		gotSha = sha
		return domain.Workspace{}, nil
	}
	g := &fakeGit{trackingExists: true, revParseSha: "originsha", setUpstreamErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err)
	assert.Equal(t, "originsha", gotSha)
}

// --- DetachHolder: top-level error propagation (mirrors RetryProvision) ---

func TestDetachHolder_GetWorkspaceError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.DetachHolder(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

func TestDetachHolder_RepoPathError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow(), fakeHome())
	_, err := uc.DetachHolder(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

func TestDetachHolder_CrowbarHomeError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	badHome := func() (string, error) { return "", errBoom }
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), badHome)
	_, err := uc.DetachHolder(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

func TestDetachHolder_HolderResolveError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	g := &fakeGit{worktreeListErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.DetachHolder(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
}

// TestDetachHolder_HomeLookupError proves that when the freed holder is the
// repo home, a failure LISTING workspaces (to find the home row) surfaces —
// the detach already happened, but the home's branch is never cleared blind.
func TestDetachHolder_HomeLookupError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ListFn = func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom }
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.DetachHolder(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, g.ops(), "DetachWorktree", "the detach itself must have already run")
}

// TestDetachHolder_HomeNotFound_SkipsClearBranch proves that when the repo has
// no default workspace to clear (an edge case, not the common case), DetachHolder
// still proceeds to provision the placeholder instead of failing.
func TestDetachHolder_HomeNotFound_SkipsClearBranch(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ListFn = func(_ context.Context) ([]domain.Workspace, error) { return nil, nil }
	clearCalled := false
	ws.ClearBranchFn = func(id string) (domain.Workspace, error) {
		clearCalled = true
		return domain.Workspace{ID: id}, nil
	}
	provisioned := false
	ws.ProvisionInPlaceFn = func(id, path, sha string) (domain.Workspace, error) {
		provisioned = true
		return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Provisioning: domain.WorkspaceProvisioned}, nil
	}
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}}, revParseSha: "sha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.NoError(t, err)
	assert.False(t, clearCalled, "no home row to clear")
	assert.True(t, provisioned, "provisioning still proceeds")
}

func TestDetachHolder_ClearBranchError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{{ID: "home", RepoID: "r1", IsDefault: true}}, nil
	}
	ws.ClearBranchFn = func(_ string) (domain.Workspace, error) { return domain.Workspace{}, errBoom }
	provisioned := false
	ws.ProvisionInPlaceFn = func(_, _, _ string) (domain.Workspace, error) {
		provisioned = true
		return domain.Workspace{}, nil
	}
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
	assert.False(t, provisioned, "no provision attempt when the home branch cannot be cleared")
}

// TestRetryProvision_FreeBranch_ProvisionsInPlace proves Retry on a placeholder
// (free after resolution) materialises a worktree, records the fork point, and
// provisions the SAME id in place (spec §3.3).
func TestRetryProvision_FreeBranch_ProvisionsInPlace(t *testing.T) {
	g := &fakeGit{revParseSha: "forksha", worktrees: nil} // no holder → Free
	var gotID, gotPath, gotSha string
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
				Provisioning: domain.WorkspacePlaceholder,
			}, nil
		},
		ProvisionInPlaceFn: func(id, path, sha string) (domain.Workspace, error) {
			gotID, gotPath, gotSha = id, path, sha
			return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Status: domain.WorkspaceStatusLocked, Provisioning: domain.WorkspaceProvisioned}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	out, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err)
	assert.Equal(t, "ph", gotID, "provisions the SAME id in place")
	assert.NotEmpty(t, gotPath)
	assert.Equal(t, "forksha", gotSha)
	assert.Equal(t, domain.WorkspaceStatusLocked, out.Status)
	assert.Contains(t, g.ops(), "WorktreeAdd")
}

// TestRetryProvision_StillHeld_ReturnsError proves a Retry while the branch is
// still held by the home returns ErrBranchStillHeld and does NOT provision.
func TestRetryProvision_StillHeld_ReturnsError(t *testing.T) {
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	provisionCalled := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
				Provisioning: domain.WorkspacePlaceholder,
			}, nil
		},
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) {
			provisionCalled = true
			return domain.Workspace{}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, hierarchy.ErrBranchStillHeld)
	assert.False(t, provisionCalled, "no provision while the branch is still held")
}

// TestDetachHolder_Home_ClearsBranchThenProvisions proves detaching a home
// holder detaches the repo folder, clears the home row's branch via ClearBranch,
// then provisions the placeholder in place (spec §3.5/B6). The second
// holder.Resolve (inside RetryProvision) sees the freed branch.
func TestDetachHolder_Home_ClearsBranchThenProvisions(t *testing.T) {
	// First Resolve: home holds develop. After DetachWorktree the fake frees it,
	// so RetryProvision's Resolve sees Free.
	g := &fakeGit{
		revParseSha:            "forksha",
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
		addConflictUntilDetach: nil,
	}
	clearedID := ""
	provisioned := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
				Provisioning: domain.WorkspacePlaceholder,
			}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "home", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true, Provisioning: domain.WorkspaceShared},
			}, nil
		},
		ClearBranchFn: func(id string) (domain.Workspace, error) {
			clearedID = id
			return domain.Workspace{ID: id}, nil
		},
		ProvisionInPlaceFn: func(id, path, sha string) (domain.Workspace, error) {
			provisioned = true
			return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Status: domain.WorkspaceStatusLocked, Provisioning: domain.WorkspaceProvisioned}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.NoError(t, err)
	assert.Contains(t, g.ops(), "DetachWorktree")
	assert.Equal(t, "home", clearedID, "the home row's branch is cleared")
	assert.True(t, provisioned, "the placeholder is provisioned after the detach")
}

// TestDetachHolder_DetachFails_NoPartialState proves a detach failure
// (mid-merge/rebase) surfaces cleanly: no ClearBranch, no provision.
func TestDetachHolder_DetachFails_NoPartialState(t *testing.T) {
	g := &fakeGit{
		worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
		detachErr: errors.New("fatal: cannot detach while merging"),
	}
	cleared := false
	provisioned := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
				Provisioning: domain.WorkspacePlaceholder,
			}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "home", RepoID: "r1", IsDefault: true}}, nil
		},
		ClearBranchFn:      func(_ string) (domain.Workspace, error) { cleared = true; return domain.Workspace{}, nil },
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) { provisioned = true; return domain.Workspace{}, nil },
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.Error(t, err)
	assert.False(t, cleared, "no ClearBranch after a failed detach")
	assert.False(t, provisioned, "no provision after a failed detach")
}

// TestRetryProvision_HeldByManaged_ReturnsError proves a Retry while ANOTHER
// Crowbar-managed worktree holds the branch refuses with its own sentinel rather
// than letting `git worktree add` fail deep in materialize. It is NOT
// ErrBranchStillHeld: detaching is not on offer, because freeing the branch
// would strand the workspace that owns it. This became reachable when import
// stopped silently dropping a protected branch held by a managed worktree that
// outlived its repo row — exactly the row that lands here.
func TestRetryProvision_HeldByManaged_ReturnsError(t *testing.T) {
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{
		{Path: "/tmp/crowbar-test/projects/p1/r1/develop/worktree", Branch: "develop"},
	}}
	provisionCalled := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo",
				Provisioning: domain.WorkspacePlaceholder,
			}, nil
		},
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) {
			provisionCalled = true
			return domain.Workspace{}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")

	require.ErrorIs(t, err, hierarchy.ErrBranchHeldByManagedWorkspace)
	require.NotErrorIs(t, err, hierarchy.ErrBranchStillHeld,
		"a managed holder cannot be detached, so it must not offer that remedy")
	assert.Contains(t, err.Error(), "/tmp/crowbar-test/projects/p1/r1/develop/worktree",
		"the refusal names the worktree holding the branch")
	assert.False(t, provisionCalled, "no provision while the branch is held")
}
