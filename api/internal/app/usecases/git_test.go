package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func newGitUsecase(
	t *testing.T,
) (
	*mocks.GitOpsEngine,
	*mocks.WorkspaceSyncer,
	usecases.GitUsecase,
) {
	t.Helper()
	git := mocks.NewGitOpsEngine()
	syncer := mocks.NewWorkspaceSyncer()
	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	uc := usecases.NewGitUsecase(git, syncer)
	return git, syncer, uc
}

// --- read surface ---

func TestGitUsecase_Status_PassThrough_NoSync(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()

	var gotPath string
	git.StatusFn = func(_ context.Context, repoPath string) (gitdomain.GitStatus, error) {
		gotPath = repoPath
		return gitdomain.GitStatus{Branch: "main"}, nil
	}

	st, err := uc.Status(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "/repo/x", gotPath)
	assert.Equal(t, "main", st.Branch)
	assert.False(t, syncer.Synced)
}

func TestGitUsecase_Status_WorkspaceError(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	git.StatusFn = func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
		return gitdomain.GitStatus{}, nil
	}

	_, err := uc.Status(ctx, "w1")
	assert.Error(t, err)
}

func TestGitUsecase_Status_EngineError(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	git.StatusFn = func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
		return gitdomain.GitStatus{}, errors.New("boom")
	}

	_, err := uc.Status(ctx, "w1")
	assert.Error(t, err)
}

func TestGitUsecase_Log_Diff_CommitDiff_Blame_Branches_Stashes_Conflicts(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()

	git.LogFn = func(_ context.Context, _ string, _, _ int) ([]gitdomain.Commit, error) {
		return []gitdomain.Commit{{Hash: "a"}}, nil
	}
	git.DiffFn = func(_ context.Context, _ string, _ bool) ([]gitdomain.FileDiff, error) {
		return []gitdomain.FileDiff{{FilePath: "f"}}, nil
	}
	git.CommitDiffFn = func(_ context.Context, _, _ string) (gitdomain.MultiFileDiff, error) {
		return gitdomain.MultiFileDiff{}, nil
	}
	git.BlameFn = func(_ context.Context, _, _ string) ([]gitdomain.BlameEntry, error) {
		return []gitdomain.BlameEntry{{}}, nil
	}
	git.BranchesFn = func(_ context.Context, _ string) ([]gitdomain.Branch, error) {
		return []gitdomain.Branch{{Name: "main"}}, nil
	}
	git.StashesFn = func(_ context.Context, _ string) ([]gitdomain.Stash, error) {
		return []gitdomain.Stash{{}}, nil
	}
	git.ConflictedFilesFn = func(_ context.Context, _ string) ([]string, error) {
		return []string{"f"}, nil
	}
	git.ConflictHunksFn = func(_ context.Context, _, _ string) ([]gitdomain.ConflictHunk, error) {
		return []gitdomain.ConflictHunk{{}}, nil
	}

	commits, err := uc.Log(ctx, "w1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, commits, 1)

	diff, err := uc.Diff(ctx, "w1", true)
	require.NoError(t, err)
	assert.Len(t, diff, 1)

	_, err = uc.CommitDiff(ctx, "w1", "sha")
	require.NoError(t, err)

	blame, err := uc.Blame(ctx, "w1", "f")
	require.NoError(t, err)
	assert.Len(t, blame, 1)

	branches, err := uc.Branches(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, branches, 1)

	stashes, err := uc.Stashes(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, stashes, 1)

	conf, err := uc.ConflictedFiles(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, conf, 1)

	hunks, err := uc.ConflictHunks(ctx, "w1", "f")
	require.NoError(t, err)
	assert.Len(t, hunks, 1)
}

func TestGitUsecase_ReadErrorsForward(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	boom := errors.New("boom")

	git.LogFn = func(_ context.Context, _ string, _, _ int) ([]gitdomain.Commit, error) { return nil, boom }
	git.DiffFn = func(_ context.Context, _ string, _ bool) ([]gitdomain.FileDiff, error) { return nil, boom }
	git.CommitDiffFn = func(_ context.Context, _, _ string) (gitdomain.MultiFileDiff, error) {
		return gitdomain.MultiFileDiff{}, boom
	}
	git.BlameFn = func(_ context.Context, _, _ string) ([]gitdomain.BlameEntry, error) { return nil, boom }
	git.BranchesFn = func(_ context.Context, _ string) ([]gitdomain.Branch, error) { return nil, boom }
	git.StashesFn = func(_ context.Context, _ string) ([]gitdomain.Stash, error) { return nil, boom }
	git.ConflictedFilesFn = func(_ context.Context, _ string) ([]string, error) { return nil, boom }
	git.ConflictHunksFn = func(_ context.Context, _, _ string) ([]gitdomain.ConflictHunk, error) {
		return nil, boom
	}

	_, err := uc.Log(ctx, "w1", 1, 0)
	assert.Error(t, err)
	_, err = uc.Diff(ctx, "w1", false)
	assert.Error(t, err)
	_, err = uc.CommitDiff(ctx, "w1", "s")
	assert.Error(t, err)
	_, err = uc.Blame(ctx, "w1", "f")
	assert.Error(t, err)
	_, err = uc.Branches(ctx, "w1")
	assert.Error(t, err)
	_, err = uc.Stashes(ctx, "w1")
	assert.Error(t, err)
	_, err = uc.ConflictedFiles(ctx, "w1")
	assert.Error(t, err)
	_, err = uc.ConflictHunks(ctx, "w1", "f")
	assert.Error(t, err)
}

// --- write surface ---

func TestGitUsecase_Commit_TriggersSync(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	var gotSubject string
	git.CommitFn = func(_ context.Context, _, subject, _ string) error {
		gotSubject = subject
		return nil
	}

	err := uc.Commit(ctx, "w1", "msg", "body", now)
	require.NoError(t, err)
	assert.Equal(t, "msg", gotSubject)
	assert.True(t, syncer.Synced)
	assert.Equal(t, "w1", syncer.SyncedID)
}

func TestGitUsecase_Commit_EngineError_NoSync(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	git.CommitFn = func(_ context.Context, _, _, _ string) error { return errors.New("boom") }

	err := uc.Commit(ctx, "w1", "m", "b", time.Now())
	assert.Error(t, err)
	assert.False(t, syncer.Synced)
}

func TestGitUsecase_Commit_WorkspaceError(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	git.CommitFn = func(_ context.Context, _, _, _ string) error { return nil }

	err := uc.Commit(ctx, "w1", "m", "b", time.Now())
	assert.Error(t, err)
}

func TestGitUsecase_AllWriteOps_TriggerSync(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	git.AnyWriteOK()

	ops := []func() error{
		func() error { return uc.StageFile(ctx, "w1", "f", now) },
		func() error { return uc.StageHunk(ctx, "w1", "f", "h", now) },
		func() error { return uc.UnstageFile(ctx, "w1", "f", now) },
		func() error { return uc.UnstageHunk(ctx, "w1", "f", "h", now) },
		func() error { return uc.Discard(ctx, "w1", "f", now) },
		func() error { return uc.Push(ctx, "w1", now) },
		func() error { return uc.Fetch(ctx, "w1", now) },
		func() error { return uc.Pull(ctx, "w1", "merge", now) },
		func() error { return uc.CreateBranch(ctx, "w1", "b", "", true, now) },
		func() error { return uc.RenameBranch(ctx, "w1", "a", "b", now) },
		func() error { return uc.DeleteBranch(ctx, "w1", "b", now) },
		func() error { return uc.SwitchBranch(ctx, "w1", "b", now) },
		func() error { return uc.StashPush(ctx, "w1", "m", now) },
		func() error { return uc.StashApply(ctx, "w1", "0", now) },
		func() error { return uc.StashPop(ctx, "w1", "0", now) },
		func() error { return uc.StashDrop(ctx, "w1", "0", now) },
		func() error { return uc.Reset(ctx, "w1", "hard", "HEAD", now) },
		func() error { return uc.Merge(ctx, "w1", "b", now) },
		func() error { return uc.Rebase(ctx, "w1", "main", now) },
		func() error {
			return uc.ResolveHunk(ctx, "w1", "f", "h", gitdomain.ConflictResolution("ours"), "c", now)
		},
		func() error { return uc.OperationContinue(ctx, "w1", now) },
		func() error { return uc.OperationAbort(ctx, "w1", now) },
	}

	for i, op := range ops {
		syncer.Synced = false
		require.NoError(t, op(), "op %d", i)
		assert.True(t, syncer.Synced, "op %d did not sync", i)
	}
}

func TestGitUsecase_WriteOp_EngineError(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	git.StageFileFn = func(_ context.Context, _, _ string) error { return errors.New("boom") }

	err := uc.StageFile(ctx, "w1", "f", time.Now())
	assert.Error(t, err)
	assert.False(t, syncer.Synced)
}

func TestGitUsecase_WriteOp_ResyncError(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	git.AnyWriteOK()
	syncer.SyncFn = func(_ context.Context, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	err := uc.Commit(ctx, "w1", "m", "b", time.Now())
	assert.Error(t, err)
}

func TestGitUsecase_ReadOps_WorkspaceError(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	git.DiffFn = func(_ context.Context, _ string, _ bool) ([]gitdomain.FileDiff, error) { return nil, nil }
	git.CommitDiffFn = func(_ context.Context, _, _ string) (gitdomain.MultiFileDiff, error) {
		return gitdomain.MultiFileDiff{}, nil
	}
	git.LogFn = func(_ context.Context, _ string, _, _ int) ([]gitdomain.Commit, error) { return nil, nil }
	git.BlameFn = func(_ context.Context, _, _ string) ([]gitdomain.BlameEntry, error) { return nil, nil }
	git.BranchesFn = func(_ context.Context, _ string) ([]gitdomain.Branch, error) { return nil, nil }
	git.StashesFn = func(_ context.Context, _ string) ([]gitdomain.Stash, error) { return nil, nil }
	git.ConflictedFilesFn = func(_ context.Context, _ string) ([]string, error) { return nil, nil }
	git.ConflictHunksFn = func(_ context.Context, _, _ string) ([]gitdomain.ConflictHunk, error) {
		return nil, nil
	}

	_, err := uc.Diff(ctx, "w1", false)
	assert.Error(t, err)
	_, err = uc.CommitDiff(ctx, "w1", "s")
	assert.Error(t, err)
	_, err = uc.Log(ctx, "w1", 1, 0)
	assert.Error(t, err)
	_, err = uc.Blame(ctx, "w1", "f")
	assert.Error(t, err)
	_, err = uc.Branches(ctx, "w1")
	assert.Error(t, err)
	_, err = uc.Stashes(ctx, "w1")
	assert.Error(t, err)
	_, err = uc.ConflictedFiles(ctx, "w1")
	assert.Error(t, err)
	_, err = uc.ConflictHunks(ctx, "w1", "f")
	assert.Error(t, err)
}
