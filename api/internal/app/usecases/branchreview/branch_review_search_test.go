package branchreview_test

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestBranchReview_SearchDiff_UsesTheSameDiffRefAsGetFiles(t *testing.T) {
	ctx := context.Background()

	var gotRepo, gotRef, gotQuery string
	var gotOpts gitdomain.SearchOpts
	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewSearchFn: func(_ context.Context, repoPath, ref, query string, opts gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) {
			gotRepo, gotRef, gotQuery, gotOpts = repoPath, ref, query, opts
			return []gitdomain.SearchHit{
				{Path: "a.go", Side: gitdomain.SearchSideNew, LineNumber: 12, Preview: "todo"},
			}, true, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	opts := gitdomain.SearchOpts{Regex: true, CaseSensitive: true, Limit: 25}
	hits, truncated, err := uc.SearchDiff(ctx, "ws1", "", "to.o", opts)
	require.NoError(t, err)

	assert.Equal(t, "/wt", gotRepo)
	assert.Equal(t, "fork1", gotRef)
	assert.Equal(t, "to.o", gotQuery)
	assert.Equal(t, opts, gotOpts)
	assert.True(t, truncated)
	require.Len(t, hits, 1)
	assert.Equal(t, 12, hits[0].LineNumber)
}

// A find-as-you-type box submits every half-finished pattern the user passes
// through, so a broken one is a client mistake (400), not a daemon failure
// (500) — and it must be rejected before a git subprocess is spawned.
func TestBranchReview_SearchDiff_InvalidRegex_IsInvalidArgumentAndNeverReachesGit(t *testing.T) {
	ctx := context.Background()

	called := false
	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewSearchFn: func(_ context.Context, _, _, _ string, _ gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) {
			called = true
			return nil, false, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, _, err := uc.SearchDiff(ctx, "ws1", "", "a(b", gitdomain.SearchOpts{Regex: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.False(t, called)
}

// The same string is a perfectly ordinary literal, and literal mode quotes it
// before compiling, so it must not be rejected as a broken pattern.
func TestBranchReview_SearchDiff_LiteralModeAcceptsRegexMetacharacters(t *testing.T) {
	ctx := context.Background()

	called := false
	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewSearchFn: func(_ context.Context, _, _, _ string, _ gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) {
			called = true
			return nil, false, nil
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, _, err := uc.SearchDiff(ctx, "ws1", "", "a(b", gitdomain.SearchOpts{})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestBranchReview_SearchDiff_MissingWorkspace_IsNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, asynxModels.ErrNotFound
		},
	}
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), &mockGitEngine{})

	_, _, err := uc.SearchDiff(ctx, "missing", "", "todo", gitdomain.SearchOpts{Limit: 10})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestBranchReview_SearchDiff_EngineFailurePropagates(t *testing.T) {
	ctx := context.Background()

	gitEng := &mockGitEngine{
		//nolint:lll // the stub mirrors the engine signature; wrapping hides which method it stands in for.
		ReviewSearchFn: func(_ context.Context, _, _, _ string, _ gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error) {
			return nil, false, errors.New("git boom")
		},
	}
	uc := newTestUsecase(outlineWorkspaceMock(), noopThreads(), mocks.NewRepositoryStore(), gitEng)

	_, _, err := uc.SearchDiff(ctx, "ws1", "", "todo", gitdomain.SearchOpts{Limit: 10})
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
	assert.NotErrorIs(t, err, apperr.ErrInvalidArgument)
}
