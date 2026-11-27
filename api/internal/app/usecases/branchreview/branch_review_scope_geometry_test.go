package branchreview_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type geometryCounts struct {
	outlines  int
	statuses  int
	revParses int
	files     int
}

func countGeometry(
	g *mockGitEngine,
	counts *geometryCounts,
) {
	g.ReviewOutlineFn = func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) {
		counts.outlines++
		return sampleOutline(), nil
	}
	g.ReviewFilesFn = func(_ context.Context, _, _ string, _ []string) ([]gitdomain.ReviewFileSummary, error) {
		counts.files++
		return []gitdomain.ReviewFileSummary{{Path: "a.go"}}, nil
	}
	g.StatusFn = func(_ context.Context, _ string) (gitdomain.GitStatus, error) {
		counts.statuses++
		return gitdomain.GitStatus{}, nil
	}
	g.RevParseFn = func(_ context.Context, _, _ string) (string, error) {
		counts.revParses++
		return "head1", nil
	}
}

func TestGetScope_GeometryCostsOneStatusAndOneOutline(
	t *testing.T,
) {
	var counts geometryCounts
	wsMock, gitEng, mergeBases := newScopeFixture(t, inSyncMergeBase)
	countGeometry(gitEng, &counts)
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), scopeFixtureChild())
	require.NoError(t, err)
	require.NotEmpty(t, scope.Outline, "the scope must carry the geometry it was asked for")

	withGeometry := len(*mergeBases)
	assert.Equal(t, 1, counts.statuses,
		"the status the file list already took is what says whether the outline is cacheable; "+
			"taking a second one is a whole subprocess spent to re-learn it")
	assert.Equal(t, 1, counts.outlines, "the geometry must be streamed once")
	assert.Equal(t, 1, counts.files)
	assert.Equal(t, 1, counts.revParses,
		"one rev-parse, for the cache key HEAD — a second would mean the key was built twice")

	counts = geometryCounts{}
	*mergeBases = nil
	_, err = uc.GetFiles(context.Background(), "child", "")
	require.NoError(t, err)

	assert.Equal(t, len(*mergeBases), withGeometry,
		"carrying the geometry must not resolve the diff ref a second time")
	assert.Equal(t, 0, counts.outlines,
		"GetFiles is the sidebar's read and wants no geometry; it must not pay for one")
}

func TestGetScope_GeometryIsCachedLikeGetOutlines(
	t *testing.T,
) {
	var counts geometryCounts
	wsMock, gitEng, _ := newScopeFixture(t, inSyncMergeBase)
	countGeometry(gitEng, &counts)
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)
	ctx := context.Background()

	first, err := uc.GetScope(ctx, scopeFixtureChild())
	require.NoError(t, err)
	second, err := uc.GetScope(ctx, scopeFixtureChild())
	require.NoError(t, err)

	assert.Equal(t, 1, counts.outlines, "a clean tree at the same HEAD must not re-stream the diff")
	assert.Equal(t, first.Outline, second.Outline)

	viaOutline, err := uc.GetOutline(ctx, "child", "")
	require.NoError(t, err)
	assert.Equal(t, 1, counts.outlines,
		"GetOutline must hit the entry the scope read populated, not stream a second one")
	assert.Equal(t, first.Outline, viaOutline)
}

func TestGetScope_GeometryMatchesGetOutlineAgainstRealGit(
	t *testing.T,
) {
	uc, ws, _ := newDivergedWorkspaceFixture(t)
	ctx := context.Background()

	scope, err := uc.GetScope(ctx, ws)
	require.NoError(t, err)
	outline, err := uc.GetOutline(ctx, ws.ID, "")
	require.NoError(t, err)

	require.NotEmpty(t, scope.Outline,
		"the fixture's child commit changes a file, so its geometry cannot be empty — "+
			"an empty one here would make the comparison below vacuous")
	assert.Equal(t, outline, scope.Outline)

	described := map[string]bool{}
	for _, f := range scope.Outline {
		described[f.Path] = true
	}
	for _, f := range scope.Files {
		assert.True(t, described[f.Path],
			"%s is in the review's file list but not in its geometry", f.Path)
	}
}
