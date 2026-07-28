package branchreview_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestBranchReview_GetPatch_UsesTheSameDiffRefAsGetFiles(t *testing.T) {
	ctx := context.Background()

	var gotRepo, gotRef, gotPath string
	var gotMax int
	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewFilePatchFn: func(_ context.Context, repoPath, ref, path string, maxLines int, w io.Writer) (int, bool, error) {
			gotRepo, gotRef, gotPath, gotMax = repoPath, ref, path, maxLines
			_, _ = io.WriteString(w, "diff --git a/a.go b/a.go\n")
			return 1, false, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	var out bytes.Buffer
	lines, truncated, err := uc.GetPatch(ctx, "ws1", "", "a.go", 50, &out)
	require.NoError(t, err)

	assert.Equal(t, "/wt", gotRepo)
	assert.Equal(t, "fork1", gotRef)
	assert.Equal(t, "a.go", gotPath)
	assert.Equal(t, 50, gotMax)
	assert.Equal(t, 1, lines)
	assert.False(t, truncated)
	assert.Equal(t, "diff --git a/a.go b/a.go\n", out.String())
}

// The engine already guards an empty path (diff.ErrEmptyPatchPath) because
// ":(top,literal)" alone matches the top of the tree and would silently turn the
// one query guaranteed to be O(one file) into a read of the whole branch diff.
// The usecase repeats the guard so the request dies before a subprocess exists.
func TestBranchReview_GetPatch_EmptyPath_IsInvalidArgumentAndNeverReachesGit(t *testing.T) {
	ctx := context.Background()

	called := false
	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewFilePatchFn: func(_ context.Context, _, _, _ string, _ int, _ io.Writer) (int, bool, error) {
			called = true
			return 0, false, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	var out bytes.Buffer
	_, _, err := uc.GetPatch(ctx, "ws1", "", "", 0, &out)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.False(t, called, "an empty path must never reach the engine")
	assert.Empty(t, out.String())
}

func TestBranchReview_GetPatch_TruncationIsReported(t *testing.T) {
	ctx := context.Background()

	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewFilePatchFn: func(_ context.Context, _, _, _ string, _ int, w io.Writer) (int, bool, error) {
			_, _ = io.WriteString(w, "@@ -1,2 +1,2 @@\n")
			return 1, true, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	var out bytes.Buffer
	lines, truncated, err := uc.GetPatch(ctx, "ws1", "", "a.go", 1, &out)
	require.NoError(t, err)
	assert.Equal(t, 1, lines)
	assert.True(t, truncated)
}

// maxLines <= 0 is the engine's "unlimited" and must reach it verbatim rather
// than being rewritten into a cap on the way through.
func TestBranchReview_GetPatch_UnlimitedMaxLinesIsForwardedVerbatim(t *testing.T) {
	ctx := context.Background()

	gotMax := -999
	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewFilePatchFn: func(_ context.Context, _, _, _ string, maxLines int, _ io.Writer) (int, bool, error) {
			gotMax = maxLines
			return 0, false, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, _, err := uc.GetPatch(ctx, "ws1", "", "a.go", 0, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, 0, gotMax)
}

func TestBranchReview_GetPatch_MissingWorkspace_IsNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, asynxModels.ErrNotFound
		},
	}
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), &mockGitEngine{})

	_, _, err := uc.GetPatch(ctx, "missing", "", "a.go", 0, io.Discard)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestBranchReview_GetPatch_EngineFailurePropagates(t *testing.T) {
	ctx := context.Background()

	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewFilePatchFn: func(_ context.Context, _, _, _ string, _ int, _ io.Writer) (int, bool, error) {
			return 0, false, errors.New("git boom")
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, _, err := uc.GetPatch(ctx, "ws1", "", "a.go", 0, io.Discard)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
	assert.NotErrorIs(t, err, apperr.ErrInvalidArgument)
}
