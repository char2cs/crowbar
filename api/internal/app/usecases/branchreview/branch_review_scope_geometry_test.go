package branchreview_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// geometryCounts is what a scope read costs in the subprocesses the geometry
// could have duplicated. Every field is a real `git` spawn in production and each
// costs about the same whatever it computes, so the counts ARE the cost model —
// the same reasoning newScopeFixture's merge-base log is built on.
type geometryCounts struct {
	outlines  int
	statuses  int
	revParses int
	files     int
}

// countGeometry adds the four counters above to an existing mock engine — the
// one newScopeFixture already wired a merge-base recorder into, since the ref
// resolution and the geometry have to be measured on the SAME call.
//
// The status it installs reports a CLEAN tree, which is the state that makes the
// outline cacheable, and therefore the state in which a second status or
// rev-parse would be pure waste rather than a necessary re-read.
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

// TestGetScope_GeometryCostsOneStatusAndOneOutline pins what carrying the hunk
// geometry on the scope is allowed to cost.
//
// The naive way to add ranges to get_review_scope was for the tool to call
// GetOutline beside GetScope, which would re-fold the workspace aggregate,
// re-resolve the diff ref (up to three merge-base spawns), and take a SECOND
// `git status` to decide whether the outline was cacheable — every one of them a
// subprocess, and every one of them answering a question the scope read had just
// answered. Phase A of this branch removed exactly that class of duplication;
// re-introducing it to add a feature would have undone the fix.
//
// So the count is the assertion: one status, one outline stream, and the ref
// resolution the file list already paid for.
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

	// The ref resolution is the expensive half, so it is measured against the
	// call that does not want geometry at all: GetScope may not resolve the ref
	// more times than GetFiles does.
	counts = geometryCounts{}
	*mergeBases = nil
	_, err = uc.GetFiles(context.Background(), "child", "")
	require.NoError(t, err)

	assert.Equal(t, len(*mergeBases), withGeometry,
		"carrying the geometry must not resolve the diff ref a second time")
	assert.Equal(t, 0, counts.outlines,
		"GetFiles is the sidebar's read and wants no geometry; it must not pay for one")
}

// TestGetScope_GeometryIsCachedLikeGetOutlines is the other half of the cost
// story: the geometry rides the SAME cache GetOutline uses, so a second scope
// read of an unmoved clean tree does not re-stream the diff.
//
// It matters because get_review_scope is paginated: walking a 400-file review is
// several calls, and a per-page diff stream would make the geometry cost scale
// with the number of pages read rather than with the diff.
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

	// And the entry is the one GetOutline serves from, which is what makes the
	// two answers the same answer rather than two answers that happen to agree.
	viaOutline, err := uc.GetOutline(ctx, "child", "")
	require.NoError(t, err)
	assert.Equal(t, 1, counts.outlines,
		"GetOutline must hit the entry the scope read populated, not stream a second one")
	assert.Equal(t, first.Outline, viaOutline)
}

// TestGetScope_GeometryMatchesGetOutlineAgainstRealGit is the anti-regression pin
// against real git plumbing rather than a stub.
//
// It is the invariant the agent surface rests on: post_review_comment validates
// every anchor against GetOutline, and get_review_scope now TELLS a model where
// to anchor from GetScope. If those two could differ, the scope would advertise
// ranges its own validator refuses — which is worse than advertising none,
// because it looks actionable.
//
// The fixture is the rebased child GetBase's own test uses, where the live merge
// base and the recorded fork point genuinely diverge, so an implementation that
// quietly resolved the geometry against a different ref would show up here.
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

	// Every file the scope lists as changed is a file the geometry describes,
	// which is what a model reading the two side by side depends on.
	described := map[string]bool{}
	for _, f := range scope.Outline {
		described[f.Path] = true
	}
	for _, f := range scope.Files {
		assert.True(t, described[f.Path],
			"%s is in the review's file list but not in its geometry", f.Path)
	}
}
