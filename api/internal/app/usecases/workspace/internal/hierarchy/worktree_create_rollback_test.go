package hierarchy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestCreateChild_NoRepoPath_CreatesRowDirectly(t *testing.T) {
	g := &fakeGit{}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", Branch: "feature/x", ParentID: "w-parent",
		ForceLocked: true,
	})
	require.NoError(t, err)
	assert.Empty(t, g.calls, "a repo with no on-disk path must never touch git")
	assert.Equal(t, "r1", created.RepoID)
	assert.Equal(t, "p1", created.ProjectID)
	assert.Equal(t, "w-parent", created.ParentID)
	assert.Equal(t, "feature/x", created.Branch)
	assert.True(t, created.Protected, "ForceLocked must carry straight through")
	assert.Empty(t, created.WorktreePath)
}

func TestCreateChild_BranchExistsCheckListError(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, errBoom },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "feature/x", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
}

func TestCreateChild_MainWorktreeAdoptedCheckListError(t *testing.T) {
	calls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			calls++
			if calls == 1 {
				return nil, nil
			}
			return nil, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "main", ParentBranch: "main",
	})
	require.ErrorIs(t, err, errBoom)
	assert.Equal(t, 2, calls, "both list checks must have run")
}

func TestCreateChild_ResolveSlugRepoLookupError(t *testing.T) {
	g := &fakeGit{}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{err: errBoom}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "feature/x", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom)
	assert.Empty(t, g.calls, "no worktree add before the slug can even be resolved")
}

func TestCreateChild_ResolveSlugRepoNotFound(t *testing.T) {
	g := &fakeGit{}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "feature/x", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestCreateChild_SiblingWorktreePaths_NotADirectory(t *testing.T) {
	home := t.TempDir()
	slugParent := filepath.Join(home, "projects", "p1")
	require.NoError(t, os.MkdirAll(slugParent, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(slugParent, "repo"), []byte("not a dir"), 0o644))

	g := &fakeGit{}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{},
		newNow(), func() (string, error) { return home, nil })

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "feature/x", ParentBranch: "develop",
	})
	require.Error(t, err)
	assert.Empty(t, g.calls, "no worktree add when the sibling scan itself fails")
}

func TestCreateChild_EmptyProjectID_RejectsWithInvalidArgument(t *testing.T) {
	g := &fakeGit{}
	uc := hierarchy.New(&fakeWorkspace{}, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "", RepoPath: "/repo", Branch: "feature/x", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestCreateChild_ParentOriginTipUnresolved_FallsBackToLocalTip(t *testing.T) {
	g := &fakeGit{
		remoteExistsByBranch: map[string]bool{"develop": true},
		revParseErr:          errBoom,
		addStartSha:          "localfork",
	}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "my-feature", ParentBranch: "develop",
	})
	require.NoError(t, err, "an unresolved origin parent tip must fall back, not fail creation")
	assert.Equal(t, []string{"/repo", created.WorktreePath, "my-feature", "develop"}, g.calls[len(g.calls)-1].args,
		"WorktreeAddBranch must fork from the LOCAL parent ref name, not a resolved sha")
	assert.Equal(t, "localfork", created.ForkPointSha)
}

func TestCreateChild_CheckoutSetUpstreamError_IsBestEffort(t *testing.T) {
	g := &fakeGit{remoteExists: true, revParseSha: "remotefork", setUpstreamErr: errBoom}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "feature/x", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, "remotefork", created.ForkPointSha)
}

func TestCreateChild_CleanupBestEffort_WhenWorktreeRemoveAndBranchDeleteBothFail(t *testing.T) {
	g := &fakeGit{addStartSha: "sha", removeErr: errBoom2, deleteErr: errBoom2}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "feature/x", ParentID: "w-parent", ParentBranch: "develop",
	})
	require.ErrorIs(t, err, errBoom, "the real row-create error must surface")
	assert.NotErrorIs(t, err, errBoom2, "a cleanup failure must never mask the real error")
	assert.Contains(t, g.ops(), "WorktreeRemove", "cleanup must still attempt the worktree removal")
	assert.Contains(t, g.ops(), "ForceDeleteBranch", "cleanup must still attempt the branch delete")
}

func TestCreateChild_RollsBackDetachWhenRetryFails_ReattachAlsoFails(t *testing.T) {
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: errBoom2,
		worktreeAddErr:         errBoom,
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
		checkoutErr:            errBoom2,
	}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true, Provisioning: domain.WorkspaceShared},
			}, nil
		},
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			t.Fatal("Create must not run when the worktree add still fails after the detach")
			return domain.Workspace{}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.ErrorIs(t, err, errBoom, "the original worktree-add failure must surface")
	assert.NotErrorIs(t, err, errBoom2, "a failed rollback re-attach must not mask the real error")
	assert.Contains(t, g.ops(), "DetachWorktree", "the detach was attempted")
	assert.Contains(t, g.ops(), "CheckoutBranch", "the re-attach must still be attempted, even though it fails")
}
