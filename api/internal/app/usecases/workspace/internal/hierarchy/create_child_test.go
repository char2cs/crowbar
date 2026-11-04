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
	// Child decision first: local remote-tracking ref (absent) then the live
	// remote query (absent) → create local. Only then is the parent start point
	// resolved (develop → not on remote → fork from the local ref).
	assert.Equal(t, []string{"RemoteTrackingBranchExists", "RemoteBranchExists", "RemoteBranchExists", "WorktreeAddBranch"}, g.ops())
	assert.Equal(t, []string{"/repo", created.WorktreePath, "feature/x", "develop"}, g.calls[3].args)
}

// TestCreateChild_CleansUpWorktreeOnCreateFailure proves H17: if the workspace
// row fails to persist after the worktree + branch were created on disk, both are
// cleaned up best-effort so a fresh-wsID retry is not blocked by the orphaned
// branch and the worktree dir does not dangle.
func TestCreateChild_CleansUpWorktreeOnCreateFailure(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
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

// TestCreateChild_DetachesMainToFreeDefaultBranch proves importing the default
// branch (held by the unmanaged main folder) detaches the main folder to free
// the branch, then retries the worktree add — yielding a real managed worktree.
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

// TestCreateChild_RollsBackDetachWhenRetryFails proves that if the worktree add
// still fails after the detach, the main folder is re-attached to its branch so
// the repo folder is never left stranded on a detached HEAD.
func TestCreateChild_RollsBackDetachWhenRetryFails(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktreeAddErr:         errBoom, // even after the detach, the add fails
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

// TestCreateChild_RejectsDuplicateBranch_ChildPath proves a NON-default branch
// that already has a workspace is rejected cleanly (not a raw git "branch
// exists" error) before git worktree add runs.
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

// TestCreateChild_DefaultsOwnWorktreeFromParent proves the taxonomy default
// rule (model spec §4.1): a create that leaves OwnWorktree unset and RepoPath
// blank inherits "owns a worktree" from a parent that itself has one — a
// locked branch — resolving RepoID/ProjectID/RepoPath/RemoteURL/ParentBranch
// from that parent's own repo rather than requiring the caller to supply them.
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

// TestCreateChild_BubbleUnderBubbleStaysBubble proves the same default rule's
// other side: a parent that owns no worktree of its own (WorktreePath == "")
// defaults its child to no worktree too, even when the child's repo resolves
// to a real one — "false sets no WorkspaceID and does no git work at all".
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

// TestCreateChild_UsesHolderResolveForDetach proves the detach path goes through
// the shared holder primitive: on the "already used by worktree" conflict it
// prunes (holder.Resolve step 1) and lists, sees the main folder holds the
// branch (held-by-home), detaches, and retries — one unified mechanism (spec §5).
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
