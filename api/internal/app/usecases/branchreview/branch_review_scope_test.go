package branchreview_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type mergeBaseCall struct {
	a string
	b string
}

func scopeFixtureChild() domain.Workspace {
	return domain.Workspace{
		ID:           "child",
		RepoID:       "repo1",
		Branch:       "feat/child",
		WorktreePath: "/wt/child",
		ParentID:     "parent",
		Provisioning: domain.WorkspaceProvisioned,
	}
}

func newScopeFixture(
	t *testing.T,
	mergeBase func(a, b string) (string, error),
) (*mockWorkspace, *mockGitEngine, *[]mergeBaseCall) {
	t.Helper()

	child := scopeFixtureChild()
	parent := domain.Workspace{ID: "parent", Branch: "develop"}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			switch id {
			case "child":
				return child, nil
			case "parent":
				return parent, nil
			}
			return domain.Workspace{}, errors.New("not found")
		},
	}

	calls := &[]mergeBaseCall{}
	gitEng := &mockGitEngine{
		MergeBaseFn: func(_ context.Context, _, a, b string) (string, error) {
			*calls = append(*calls, mergeBaseCall{a: a, b: b})
			return mergeBase(a, b)
		},
	}
	return wsMock, gitEng, calls
}

func inSyncMergeBase(
	_ string,
	_ string,
) (string, error) {
	return "base-sha", nil
}

func TestGetScope_ResolvesTheDiffRefOnce(
	t *testing.T,
) {
	wsMock, gitEng, calls := newScopeFixture(t, inSyncMergeBase)
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), scopeFixtureChild())
	require.NoError(t, err)
	assert.Equal(t, "base-sha", scope.Base)

	viaScope := len(*calls)
	*calls = nil

	_, err = uc.GetFiles(context.Background(), "child", "")
	require.NoError(t, err)
	viaFiles := len(*calls)

	assert.Equal(t, viaFiles, viaScope,
		"GetScope must cost exactly what the file list alone costs: the ref was always "+
			"resolved on the way to it and merely discarded")
	assert.NotZero(t, viaScope,
		"a fixture where nothing reaches git would satisfy the equality above vacuously")
}

func TestGetScope_ReportsTheRefItsFilesWereDiffedFrom(
	t *testing.T,
) {
	wsMock, gitEng, _ := newScopeFixture(t, inSyncMergeBase)
	var diffedRef string
	gitEng.ReviewFilesFn = func(
		_ context.Context,
		_ string,
		ref string,
		_ []string,
	) ([]gitdomain.ReviewFileSummary, error) {
		diffedRef = ref
		return []gitdomain.ReviewFileSummary{{Path: "a.go"}}, nil
	}
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), scopeFixtureChild())
	require.NoError(t, err)
	assert.Equal(t, diffedRef, scope.Base)
	require.Len(t, scope.Files, 1)
	assert.Equal(t, "a.go", scope.Files[0].Path)
}

func TestResolveDiffRef_IdenticalCandidatesSkipTheComparison(
	t *testing.T,
) {
	wsMock, gitEng, calls := newScopeFixture(t, inSyncMergeBase)
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), scopeFixtureChild())
	require.NoError(t, err)
	assert.Equal(t, "base-sha", scope.Base)

	assert.Equal(t, []mergeBaseCall{
		{a: "origin/develop", b: "HEAD"},
		{a: "develop", b: "HEAD"},
	}, *calls, "a candidate compared against itself must never reach git")
}

func TestResolveDiffRef_DivergentCandidatesStillCompare(
	t *testing.T,
) {
	wsMock, gitEng, calls := newScopeFixture(t, func(a, _ string) (string, error) {
		switch a {
		case "origin/develop":
			return "origin-base", nil
		case "develop":
			return "local-base", nil
		}
		return "origin-base", nil
	})
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), scopeFixtureChild())
	require.NoError(t, err)

	assert.Equal(t, "local-base", scope.Base,
		"the merge base closest to HEAD must still win when the candidates differ")
	assert.Equal(t, []mergeBaseCall{
		{a: "origin/develop", b: "HEAD"},
		{a: "develop", b: "HEAD"},
		{a: "origin-base", b: "local-base"},
	}, *calls)
}

func TestGetScope_AgreesWithGetFiles(
	t *testing.T,
) {
	uc, ws, wantRef := newDivergedWorkspaceFixture(t)
	ctx := context.Background()

	scope, err := uc.GetScope(ctx, ws)
	require.NoError(t, err)

	files, err := uc.GetFiles(ctx, ws.ID, "")
	require.NoError(t, err)

	assert.Equal(t, wantRef, scope.Base)
	assert.Equal(t, files, scope.Files)
	assert.NotEqual(t, ws.ForkPointSha, scope.Base,
		"GetScope must not shortcut to the frozen fork point once the base branch has advanced past it")
}
