package hierarchy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestCreateChild_RecordsForkPointAndLocked(t *testing.T) {
	g := &fakeGit{addStartSha: "sha123"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{protected: []string{"main"}}, &fakeRepoStore{}, newNow(), fakeHome())

	out, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ID)
	assert.Equal(t, "sha123", created.ForkPointSha)
	assert.False(t, created.Protected)
	assert.Equal(t, "w-parent", created.ParentID)
	assert.Equal(t, []string{"RemoteTrackingBranchExists", "RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[3].args)
}

func TestCreateChild_CleansUpWorktreeOnCreateFailure(t *testing.T) {
	ctx, hangUp := context.WithCancel(t.Context())
	g := &fakeGit{addStartSha: "sha"}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			hangUp()
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(ctx, hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
	ops := g.ops()
	assert.Contains(t, ops, "WorktreeRemove", "failed create must remove the orphaned worktree")
	assert.Contains(t, ops, "ForceDeleteBranch", "failed create must delete the orphaned branch")
}

func TestCreateChild_LocksProtectedBranch(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(
			_ context.Context,
			in workspace.CreateInput,
			_ time.Time,
		) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{protected: []string{"feature/x"}}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/repo", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git", Branch: "feature/x", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.True(t, created.Protected)
}

func TestCreateChild_DetachesMainToFreeDefaultBranch(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
	}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true, Provisioning: domain.WorkspaceShared},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.NoError(t, err, "importing the default branch must succeed by detaching the main folder")
	assert.Contains(t, g.ops(), "DetachWorktree", "the main folder must be detached to free the branch")
	var addCount int
	for _, op := range g.ops() {
		if op == "WorktreeAddAtRef" {
			addCount++
		}
	}
	assert.Equal(t, 2, addCount, "worktree add runs once (conflict), then again after the detach")
	assert.NotEqual(t, "/repo", created.WorktreePath, "the managed worktree lives outside the repo folder")
	assert.Equal(t, "forksha", created.ForkPointSha)
}

func TestCreateChild_RollsBackDetachWhenRetryFails(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktreeAddErr:         errBoom,
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
	}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true, Provisioning: domain.WorkspaceShared},
			}, nil
		},
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			t.Fatal("Create must not run when the worktree add fails")
			return domain.Workspace{}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.Error(t, err, "a persistent add failure must surface as an error")
	assert.Contains(t, g.ops(), "DetachWorktree", "the detach was attempted")
	assert.Contains(t, g.ops(), "CheckoutBranch", "the main folder must be re-attached after the failed retry")
}

func TestCreateChild_RejectsDuplicateBranch_ChildPath(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	createCalls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "ws-x", RepoID: "r1", Branch: "feature/x"}}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createCalls++
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "feature/x", ParentID: "w-parent", ParentBranch: "develop",
	})

	require.ErrorIs(t, err, hierarchy.ErrBranchWorkspaceExists)
	assert.Equal(t, 0, createCalls)
	assert.NotContains(t, g.ops(), "WorktreeAddBranch", "guard rejects before git worktree add")
}

func TestCreateChild_ProviderError(t *testing.T) {
	g := &fakeGit{addStartSha: "s"}
	ws := &fakeWorkspace{}
	uc := hierarchy.New(ws, g, &fakeProvider{err: errBoom}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{RepoPath: "/r", ProjectID: "p1", RemoteURL: "https://github.com/test/repo.git", Branch: "b"})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_CrowbarHomeError(t *testing.T) {
	g := &fakeGit{}
	badHome := func() (string, error) { return "", errBoom }
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), badHome)
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoPath: "/r", Branch: "feature/x", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
	assert.Empty(t, g.calls, "no git work runs when the home path cannot be resolved")
}

func TestCreateChild_DefaultsOwnWorktreeFromParent(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID:           id,
				RepoID:       "r1",
				ProjectID:    "p1",
				Branch:       "develop",
				WorktreePath: "/repo/worktrees/develop",
				Provisioning: domain.WorkspaceProvisioned,
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID, WorktreePath: in.WorktreePath, Provisioning: domain.WorkspaceProvisioned}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", remoteURL: "https://github.com/test/repo.git"}, newNow(), fakeHome())

	child, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		ParentID: "locked-parent",
		Branch:   "feature/x",
	})
	require.NoError(t, err)
	if child.WorktreePath == "" {
		t.Fatalf("a child of a locked row must default to owning a worktree")
	}
}

func TestCreateChild_BubbleUnderBubbleStaysBubble(t *testing.T) {
	g := &fakeGit{}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{ID: id, RepoID: "r1", ProjectID: "p1"}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID, RepoID: in.RepoID, ParentID: in.ParentID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo", remoteURL: "https://github.com/test/repo.git"}, newNow(), fakeHome())

	child, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		ParentID: "bubble-parent",
	})
	require.NoError(t, err)
	if child.WorktreePath != "" {
		t.Fatalf("a child of a workspace-less chat must default to no workspace")
	}
	assert.Equal(t, "r1", child.RepoID, "RepoID still resolves from the parent even though no worktree is created")
	assert.Empty(t, g.calls, "no git work runs when the create does not own a worktree")
}

func TestCreateChild_UsesHolderResolveForDetach(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
	}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true, Provisioning: domain.WorkspaceShared},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Contains(t, g.ops(), "WorktreePrune", "holder.Resolve prunes dead regs before classifying")
	assert.Contains(t, g.ops(), "DetachWorktree")
}
