package branchreview_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestBranchReview_GetFileSides_UsesTheSameDiffRefAsGetPatch(t *testing.T) {
	var gotRepo, gotRef, gotPath, gotOld string
	text := "a\n"
	gitEng := &mockGitEngine{
		ReviewFileSidesFn: func(_ context.Context, repoPath, ref, path, oldPath string) (gitdomain.ReviewFileSides, error) {
			gotRepo, gotRef, gotPath, gotOld = repoPath, ref, path, oldPath
			return gitdomain.ReviewFileSides{New: &text}, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	sides, err := uc.GetFileSides(context.Background(), "ws1", "", "a.go", "old.go")
	require.NoError(t, err)

	assert.Equal(t, "/wt", gotRepo)
	assert.Equal(t, "fork1", gotRef)
	assert.Equal(t, "a.go", gotPath)
	assert.Equal(t, "old.go", gotOld)
	assert.Nil(t, sides.Old)
	assert.Equal(t, "a\n", *sides.New)
}

func TestBranchReview_GetFileSides_EmptyPathNeverReachesGit(t *testing.T) {
	called := false
	gitEng := &mockGitEngine{
		ReviewFileSidesFn: func(context.Context, string, string, string, string) (gitdomain.ReviewFileSides, error) {
			called = true
			return gitdomain.ReviewFileSides{}, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetFileSides(context.Background(), "ws1", "", "", "")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.False(t, called)
}

func TestBranchReview_GetFileSides_OversizedFileIsUnprocessable(t *testing.T) {
	gitEng := &mockGitEngine{
		ReviewFileSidesFn: func(context.Context, string, string, string, string) (gitdomain.ReviewFileSides, error) {
			return gitdomain.ReviewFileSides{}, enginegit.ErrFileTooLarge
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, err := uc.GetFileSides(context.Background(), "ws1", "", "big.go", "")

	assert.ErrorIs(t, err, apperr.ErrUnprocessable)
}
