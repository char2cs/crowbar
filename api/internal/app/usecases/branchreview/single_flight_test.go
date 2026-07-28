package branchreview_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// blockingGit is a ReviewFiles stand-in that parks its first parkLimit
// invocations on a gate the test opens, so a flight can be held open for
// exactly as long as the test needs while it counts how many invocations
// actually reached the engine. Invocations past parkLimit return immediately,
// so a dedup failure surfaces as a wrong count rather than as a deadlock.
// entered is buffered to the maximum number of callers so the fake never
// blocks on an unread signal.
type blockingGit struct {
	invocations atomic.Int64
	entered     chan struct{}
	release     chan struct{}
	parkLimit   int64
	ctxSeen     context.Context
	files       []gitdomain.ReviewFileSummary
	err         error
}

func newBlockingGit(maxCallers int) *blockingGit {
	return &blockingGit{
		entered:   make(chan struct{}, maxCallers),
		release:   make(chan struct{}),
		parkLimit: int64(maxCallers),
	}
}

func (b *blockingGit) reviewFiles(
	ctx context.Context,
	_, _ string,
	_ []string,
) ([]gitdomain.ReviewFileSummary, error) {
	n := b.invocations.Add(1)
	if n > b.parkLimit {
		return slices.Clone(b.files), b.err
	}
	if n == 1 {
		b.ctxSeen = ctx
	}
	b.entered <- struct{}{}
	<-b.release
	return slices.Clone(b.files), b.err
}

func (b *blockingGit) engine() *mockGitEngine {
	return &mockGitEngine{ReviewFilesFn: b.reviewFiles}
}

// workspacesByID resolves any id to a workspace carrying that id, so one mock
// serves a test that exercises several workspaces.
func workspacesByID() *mockWorkspace {
	return &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID:           id,
				RepoID:       "repo1",
				Branch:       "feature",
				WorktreePath: "/wt/" + id,
				ForkPointSha: "fork-" + id,
			}, nil
		},
	}
}

// cancelledContext is the barrier this file uses to prove a caller has reached
// the flight registry. GetFiles registers on the flight BEFORE it observes its
// own context, so a caller handed an already-cancelled context returns only
// after it has provably attached to whatever flight was live at that moment.
// That is a real signal, unlike "the goroutine has been started".
func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestGetFiles_JoinsALiveFlight is the airtight half of the single-flight
// contract: a caller arriving while a computation is in progress attaches to it
// instead of starting a second one. The leader is provably parked inside the
// engine (<-entered) and stays parked for the whole assertion, and the joiner
// cannot return without having gone through the flight registry.
func TestGetFiles_JoinsALiveFlight(t *testing.T) {
	git := newBlockingGit(2)
	git.parkLimit = 1
	uc := newTestUsecase(workspacesByID(), noopThreads(), mocks.NewRepositoryStore(), git.engine())

	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		_, _ = uc.GetFiles(context.Background(), "ws1", "")
	}()
	<-git.entered

	_, err := uc.GetFiles(cancelledContext(), "ws1", "")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int64(1), git.invocations.Load(),
		"a caller arriving during a live flight must join it, not start a second computation")

	close(git.release)
	<-leaderDone
	assert.Equal(t, int64(1), git.invocations.Load())
}

// TestGetFiles_SingleFlightsConcurrentCallers exercises the real shape: a burst
// of N concurrent callers for one workspace. The assertion is a bound rather
// than "exactly one" because the moment a caller registers on the flight is not
// observable from outside singleflight — a caller can lose the race between
// being launched and reaching the registry, and asserting == 1 here flakes
// (confirmed at -count=5). TestGetFiles_JoinsALiveFlight pins the exact-one
// case on a signal that IS observable; this test pins that a burst collapses
// and that every caller gets the same correct answer.
func TestGetFiles_SingleFlightsConcurrentCallers(t *testing.T) {
	const callers = 8

	git := newBlockingGit(callers)
	git.files = []gitdomain.ReviewFileSummary{
		{Path: "a.go", Status: gitdomain.GitFileStatusModified, Additions: 3, Deletions: 1},
		{Path: "b.go", Status: gitdomain.GitFileStatusAdded, Additions: 7},
	}
	uc := newTestUsecase(workspacesByID(), noopThreads(), mocks.NewRepositoryStore(), git.engine())

	results := make([][]gitdomain.ReviewFileSummary, callers)
	errs := make([]error, callers)
	var launched, done sync.WaitGroup
	launched.Add(callers)
	done.Add(callers)
	for i := range callers {
		go func() {
			defer done.Done()
			launched.Done()
			results[i], errs[i] = uc.GetFiles(context.Background(), "ws1", "")
		}()
	}

	launched.Wait()
	<-git.entered
	close(git.release)
	done.Wait()

	invocations := git.invocations.Load()
	assert.Positive(t, invocations)
	assert.Less(t, invocations, int64(callers),
		"a burst of concurrent callers for one workspace must collapse into shared computations")
	for i := range callers {
		require.NoError(t, errs[i])
		assert.Equal(t, git.files, results[i], "every caller must receive the same correct answer")
	}

	// Waiters must not alias one mutable slice: one caller editing its result
	// would otherwise silently corrupt every other caller's.
	results[0][0].Additions = 9999
	assert.Equal(t, 3, results[1][0].Additions, "waiters must not share one backing array")
}

func TestGetFiles_SeparateWorkspacesDoNotShare(t *testing.T) {
	git := newBlockingGit(2)
	uc := newTestUsecase(workspacesByID(), noopThreads(), mocks.NewRepositoryStore(), git.engine())

	var done sync.WaitGroup
	done.Add(2)
	for _, wsID := range []string{"ws1", "ws2"} {
		go func() {
			defer done.Done()
			_, _ = uc.GetFiles(context.Background(), wsID, "")
		}()
	}

	// Both workspaces are provably inside the engine at the same time before the
	// gate opens, so a shared flight could not have hidden one of them.
	<-git.entered
	<-git.entered
	close(git.release)
	done.Wait()

	assert.Equal(t, int64(2), git.invocations.Load(),
		"a flight is keyed on workspace id: distinct workspaces must compute independently")
}

func TestGetFiles_ErrorPropagatesToAllWaiters(t *testing.T) {
	const callers = 6

	git := newBlockingGit(callers)
	git.err = errors.New("git: not a repository")
	uc := newTestUsecase(workspacesByID(), noopThreads(), mocks.NewRepositoryStore(), git.engine())

	errs := make([]error, callers)
	var launched, done sync.WaitGroup
	launched.Add(callers)
	done.Add(callers)
	for i := range callers {
		go func() {
			defer done.Done()
			launched.Done()
			_, errs[i] = uc.GetFiles(context.Background(), "ws1", "")
		}()
	}

	launched.Wait()
	<-git.entered
	close(git.release)
	done.Wait()

	assert.Less(t, git.invocations.Load(), int64(callers),
		"the burst must have produced waiters, or this proves nothing about propagation")
	for i := range callers {
		require.Error(t, errs[i], "a failed flight must fail every waiter, not just the leader")
		assert.ErrorContains(t, errs[i], "not a repository")
	}
}

// TestGetFiles_StartingCallerCancellationDoesNotAbortSharedWork pins why the
// shared computation runs on a detached context. The caller that happens to
// start a flight owns it for everyone behind it, so its own disconnect must not
// take the flight down with it — otherwise one aborted request would fail
// unrelated live ones.
func TestGetFiles_StartingCallerCancellationDoesNotAbortSharedWork(t *testing.T) {
	git := newBlockingGit(2)
	git.parkLimit = 1
	uc := newTestUsecase(workspacesByID(), noopThreads(), mocks.NewRepositoryStore(), git.engine())

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := uc.GetFiles(leaderCtx, "ws1", "")
		leaderErr <- err
	}()
	<-git.entered

	cancelLeader()
	assert.NoError(t, git.ctxSeen.Err(),
		"the shared computation must survive the cancellation of the caller that started it")

	close(git.release)
	require.ErrorIs(t, <-leaderErr, context.Canceled,
		"a caller must still honour its own cancellation")
	assert.Equal(t, int64(1), git.invocations.Load())
}

// TestGetFiles_NewCallAfterCompletionRecomputes pins that this is deduplication
// of in-flight work and NOT a cache: once a flight has finished, the next call
// must run the computation again or the sidebar would serve a stale file list.
func TestGetFiles_NewCallAfterCompletionRecomputes(t *testing.T) {
	git := newBlockingGit(2)
	close(git.release)
	git.files = []gitdomain.ReviewFileSummary{{Path: "a.go", Status: gitdomain.GitFileStatusModified}}
	uc := newTestUsecase(workspacesByID(), noopThreads(), mocks.NewRepositoryStore(), git.engine())

	_, err := uc.GetFiles(context.Background(), "ws1", "")
	require.NoError(t, err)
	_, err = uc.GetFiles(context.Background(), "ws1", "")
	require.NoError(t, err)

	assert.Equal(t, int64(2), git.invocations.Load(),
		"a completed flight must not serve a later call")
}
