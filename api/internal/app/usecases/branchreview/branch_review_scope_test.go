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

// mergeBaseCall records one MergeBase invocation's two revisions, which is what
// separates "the ref was resolved twice" from "the resolution is expensive": the
// duplicated work under measurement is byte-identical in its arguments.
type mergeBaseCall struct {
	a string
	b string
}

// newScopeFixture wires a child->parent workspace pair over a MergeBase stub
// whose answers are supplied per test, recording every call. Every subcommand
// here is a real subprocess in production and costs about the same whatever it
// computes, so the call log IS the cost model.
func newScopeFixture(
	t *testing.T,
	mergeBase func(a, b string) (string, error),
) (*mockWorkspace, *mockGitEngine, *[]mergeBaseCall) {
	t.Helper()

	child := domain.Workspace{
		ID:           "child",
		RepoID:       "repo1",
		Branch:       "feat/child",
		WorktreePath: "/wt/child",
		ParentID:     "parent",
	}
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

// inSyncMergeBase is the ordinary state of a branch nobody has pushed to since
// the last fetch: origin/develop and develop name the same commit, so both
// probes yield the same merge base.
func inSyncMergeBase(
	_ string,
	_ string,
) (string, error) {
	return "base-sha", nil
}

// TestGetScope_ResolvesTheDiffRefOnce pins the whole point of GetScope: the ref
// and the file list come out of ONE resolution.
//
// The assertion is a call count rather than a duration because the duplicate
// resolution was nearly free in wall time — the second merge-base asks git the
// same question the first one just did, so it hits every cache in the stack while
// still paying a full subprocess spawn. Only the count survives a warm cache.
func TestGetScope_ResolvesTheDiffRefOnce(
	t *testing.T,
) {
	wsMock, gitEng, calls := newScopeFixture(t, inSyncMergeBase)
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), "child")
	require.NoError(t, err)
	assert.Equal(t, "base-sha", scope.Base)

	viaScope := len(*calls)
	*calls = nil

	_, err = uc.GetFiles(context.Background(), "child", "")
	require.NoError(t, err)
	viaFiles := len(*calls)
	*calls = nil

	_, err = uc.GetBase(context.Background(), "child")
	require.NoError(t, err)
	viaBase := len(*calls)

	assert.Equal(t, viaFiles, viaScope,
		"GetScope must cost exactly what the file list alone costs: the ref was always "+
			"resolved on the way to it and merely discarded")
	assert.Less(t, viaScope, viaFiles+viaBase,
		"GetScope must be cheaper than the GetBase+GetFiles pair it replaces")
}

// TestGetScope_ReportsTheRefItsFilesWereDiffedFrom is the correctness half of the
// same change: one resolution cannot disagree with itself, where two independent
// ones can straddle a push to the base branch and report a file list against a
// ref they were never diffed from.
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

	scope, err := uc.GetScope(context.Background(), "child")
	require.NoError(t, err)
	assert.Equal(t, diffedRef, scope.Base)
	require.Len(t, scope.Files, 1)
	assert.Equal(t, "a.go", scope.Files[0].Path)
}

// TestResolveDiffRef_IdenticalCandidatesSkipTheComparison pins the second
// subprocess this change removes. With the base branch in sync with origin the
// two probes return the SAME sha, and comparing a sha with itself can only
// re-elect the candidate already held — merge-base(X, X) is X by definition — so
// the comparison is a whole subprocess spent to learn nothing.
//
// Two calls, not three: one probe for origin/develop, one for develop.
func TestResolveDiffRef_IdenticalCandidatesSkipTheComparison(
	t *testing.T,
) {
	wsMock, gitEng, calls := newScopeFixture(t, inSyncMergeBase)
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), "child")
	require.NoError(t, err)
	assert.Equal(t, "base-sha", scope.Base)

	assert.Equal(t, []mergeBaseCall{
		{a: "origin/develop", b: "HEAD"},
		{a: "develop", b: "HEAD"},
	}, *calls, "a candidate compared against itself must never reach git")
}

// TestResolveDiffRef_DivergentCandidatesStillCompare is the guard on the
// short-circuit above: skipping the comparison must be conditional on the
// candidates being equal, never unconditional. A local base branch ahead of
// origin yields two different merge bases and the descendant one — the local
// one, closest to HEAD — must still win, which takes the third subprocess.
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
		// merge-base(origin-base, local-base) == origin-base: origin's base is an
		// ancestor of the local one, so the local one sits closer to HEAD.
		return "origin-base", nil
	})
	uc := newTestUsecase(wsMock, noopThreads(), mocks.NewRepositoryStore(), gitEng)

	scope, err := uc.GetScope(context.Background(), "child")
	require.NoError(t, err)

	assert.Equal(t, "local-base", scope.Base,
		"the merge base closest to HEAD must still win when the candidates differ")
	assert.Equal(t, []mergeBaseCall{
		{a: "origin/develop", b: "HEAD"},
		{a: "develop", b: "HEAD"},
		{a: "origin-base", b: "local-base"},
	}, *calls)
}

// TestGetScope_AgreesWithGetBaseAndGetFiles is the anti-regression pin against
// REAL git plumbing rather than a stub: GetScope is only allowed to be cheaper,
// never to answer differently. The fixture is the rebased-child one GetBase's own
// test uses, where the live merge base and the recorded fork point genuinely
// diverge, so an implementation that quietly shortcut to the frozen fork point
// would fail here.
func TestGetScope_AgreesWithGetBaseAndGetFiles(
	t *testing.T,
) {
	uc, ws, wantRef := newDivergedWorkspaceFixture(t)
	ctx := context.Background()

	scope, err := uc.GetScope(ctx, ws.ID)
	require.NoError(t, err)

	base, err := uc.GetBase(ctx, ws.ID)
	require.NoError(t, err)
	files, err := uc.GetFiles(ctx, ws.ID, "")
	require.NoError(t, err)

	assert.Equal(t, wantRef, scope.Base)
	assert.Equal(t, base, scope.Base)
	assert.Equal(t, files, scope.Files)
	assert.NotEqual(t, ws.ForkPointSha, scope.Base,
		"GetScope must not shortcut to the frozen fork point once the base branch has advanced past it")
}
