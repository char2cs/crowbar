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

type statusProviderStub struct{}

func (statusProviderStub) GitStatus(
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func newFileUsecase(
	t *testing.T,
) (
	*mocks.FsEngine,
	*mocks.WorkspaceSyncer,
	usecases.FileUsecase,
) {
	t.Helper()
	fs := mocks.NewFsEngine()
	syncer := mocks.NewWorkspaceSyncer()
	uc := usecases.NewFileUsecase(fs, syncer)
	return fs, syncer, uc
}

func TestFileUsecase_Tree_PassesThrough(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	var gotPath string
	fs.TreeFn = func(repoPath, _ string, _ usecases.FileStatusProvider) ([]domain.FileNode, error) {
		gotPath = repoPath
		return []domain.FileNode{{Name: "a.go"}}, nil
	}

	nodes, err := uc.Tree(ctx, "w1", "", statusProviderStub{})
	require.NoError(t, err)
	assert.Equal(t, "/repo/x", gotPath)
	assert.Len(t, nodes, 1)
	assert.False(t, syncer.Synced)
}

func TestFileUsecase_Tree_WorkspaceError(t *testing.T) {
	_, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.Tree(ctx, "w1", "", statusProviderStub{})
	assert.Error(t, err)
}

func TestFileUsecase_ReadContent_PassesThrough(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	fs.ReadContentFn = func(_, _ string) (domain.FileContent, error) {
		return domain.FileContent{Content: "hi"}, nil
	}

	got, err := uc.ReadContent(ctx, "w1", "a.go")
	require.NoError(t, err)
	assert.Equal(t, "hi", got.Content)
	assert.False(t, syncer.Synced)
}

func TestFileUsecase_WriteContent_TriggersSync(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	var wrotePath, wroteContent string
	fs.WriteContentFn = func(_, filePath, content string) error {
		wrotePath = filePath
		wroteContent = content
		return nil
	}

	err := uc.WriteContent(ctx, "w1", "a.go", "data", now)
	require.NoError(t, err)
	assert.Equal(t, "a.go", wrotePath)
	assert.Equal(t, "data", wroteContent)
	assert.True(t, syncer.Synced)
	assert.Equal(t, "w1", syncer.SyncedID)
}

func TestFileUsecase_WriteContent_FsError(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	fs.WriteContentFn = func(_, _, _ string) error {
		return errors.New("boom")
	}

	err := uc.WriteContent(ctx, "w1", "a.go", "data", time.Now())
	assert.Error(t, err)
	assert.False(t, syncer.Synced)
}

func TestFileUsecase_CreateFile_TriggersSync(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	fs.CreateFileFn = func(_, _ string) error { return nil }

	require.NoError(t, uc.CreateFile(ctx, "w1", "a.go", time.Now()))
	assert.True(t, syncer.Synced)
}

func TestFileUsecase_CreateDir_TriggersSync(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	fs.CreateDirFn = func(_, _ string) error { return nil }

	require.NoError(t, uc.CreateDir(ctx, "w1", "sub", time.Now()))
	assert.True(t, syncer.Synced)
}

func TestFileUsecase_Rename_TriggersSync(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	fs.RenameFn = func(_, _, _ string) error { return nil }

	require.NoError(t, uc.Rename(ctx, "w1", "a.go", "b.go", time.Now()))
	assert.True(t, syncer.Synced)
}

func TestFileUsecase_Delete_TriggersSync(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	fs.DeleteFn = func(_, _ string) error { return nil }

	require.NoError(t, uc.Delete(ctx, "w1", "a.go", time.Now()))
	assert.True(t, syncer.Synced)
}

func TestFileUsecase_Delete_WorkspaceError(t *testing.T) {
	_, syncer, uc := newFileUsecase(t)
	ctx := context.Background()

	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	err := uc.Delete(ctx, "w1", "a.go", time.Now())
	assert.Error(t, err)
}

func TestFileUsecase_AllOps_WorkspaceError(t *testing.T) {
	_, syncer, uc := newFileUsecase(t)
	ctx := context.Background()
	now := time.Now()
	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}

	_, err := uc.ReadContent(ctx, "w1", "a.go")
	assert.Error(t, err)
	assert.Error(t, uc.WriteContent(ctx, "w1", "a.go", "d", now))
	assert.Error(t, uc.CreateFile(ctx, "w1", "a.go", now))
	assert.Error(t, uc.CreateDir(ctx, "w1", "d", now))
	assert.Error(t, uc.Rename(ctx, "w1", "a.go", "b.go", now))
}

func TestFileUsecase_WriteContent_ResyncError(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	ctx := context.Background()
	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	syncer.SyncFn = func(_ context.Context, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	fs.WriteContentFn = func(_, _, _ string) error { return nil }

	err := uc.WriteContent(ctx, "w1", "a.go", "d", time.Now())
	assert.Error(t, err)
}
