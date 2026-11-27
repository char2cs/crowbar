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

func TestRetryProvision_DeriveWorktreePathError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "", "develop")
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.RetryProvision(context.Background(), "ph")
	require.Error(t, err)
	assert.NotContains(t, g.ops(), "WorktreeAdd", "no worktree materialization when the path cannot be derived")
}

func TestRetryProvision_MaterializeError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	g := &fakeGit{worktreeAddErr: errBoom}
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

func TestRetryProvision_NotOnOrigin_RevParseFails_ForkPointNonEssential(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	var gotSha string
	provisioned := false
	ws.ProvisionInPlaceFn = func(_, _, sha string) (domain.Workspace, error) {
		gotSha = sha
		provisioned = true
		return domain.Workspace{}, nil
	}
	g := &fakeGit{revParseErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err)
	assert.True(t, provisioned)
	assert.Empty(t, gotSha, "an unresolvable fork point must not fail provisioning")
}

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

func TestDetachHolder_HomeLookupError(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ListFn = func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom }
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())
	_, err := uc.DetachHolder(context.Background(), "ph")
	require.ErrorIs(t, err, errBoom)
	assert.Contains(t, g.ops(), "DetachWorktree", "the detach itself must have already run")
}

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

func TestRetryProvision_FreeBranch_ProvisionsInPlace(t *testing.T) {
	g := &fakeGit{revParseSha: "forksha", worktrees: nil}
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

func TestDetachHolder_Home_ClearsBranchThenProvisions(t *testing.T) {
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
