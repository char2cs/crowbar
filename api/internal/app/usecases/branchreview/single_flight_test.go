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

	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
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

// TestGetFiles_SingleFlightsConcurrentCallers scales TestGetFiles_JoinsALiveFlight
// from one joiner to N-1 and asserts the exact count, not a bound.
//
// It used to launch a burst of goroutines and assert only `invocations <
// callers`, because "the caller has reached the flight registry" was thought to
// be unobservable. It is observable, and this file already names the signal:
// scopeOf calls DoChan — which registers under the group's lock before it
// returns — BEFORE the select that observes ctx.Done(), so a caller handed an
// already-cancelled context cannot return until it has provably attached. The
// return IS the barrier.
//
// The old shape flaked because launched.Done() fires BEFORE GetFiles, so
// launched.Wait() proved only that the goroutines had been scheduled. Releasing
// the leader then deleted the key, and every caller not yet inside started a
// fresh flight — 8 invocations under enough CPU contention. Attaching each
// joiner one at a time on a real barrier removes the race rather than widening
// the tolerance for it.
func TestGetFiles_SingleFlightsConcurrentCallers(t *testing.T) {
	const callers = 8

	git := newBlockingGit(callers)
	// Park only the leader. Should dedup ever break, the stray invocation
	// returns immediately and is counted, rather than parking and deadlocking
	// the test it was meant to fail.
	git.parkLimit = 1
	git.files = []gitdomain.ReviewFileSummary{
		{Path: "a.go", Status: gitdomain.GitFileStatusModified, Additions: 3, Deletions: 1},
		{Path: "b.go", Status: gitdomain.GitFileStatusAdded, Additions: 7},
	}
	uc := newTestUsecase(workspacesByID(), noopThreads(), mocks.NewRepositoryStore(), git.engine())

	var leaderFiles []gitdomain.ReviewFileSummary
	var leaderErr error
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		leaderFiles, leaderErr = uc.GetFiles(context.Background(), "ws1", "")
	}()
	<-git.entered

	for range callers - 1 {
		_, err := uc.GetFiles(cancelledContext(), "ws1", "")
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, int64(1), git.invocations.Load(),
			"every caller arriving during a live flight must join it, not start another")
	}

	close(git.release)
	<-leaderDone

	require.NoError(t, leaderErr)
	assert.Equal(t, git.files, leaderFiles, "the leader must still receive the correct answer")
	assert.Equal(t, int64(1), git.invocations.Load(),
		"a burst of callers for one workspace must collapse into exactly one computation")
}

// TestSharedScope_ClonesPerWaiter replaces the aliasing assertion the burst test
// used to carry. Waiters must not alias one mutable slice — one caller editing
// its result would silently corrupt every other caller's — but that property
// belongs to sharedScope's slices.Clone, which is a pure function. Two calls on
// one flight result prove it without racing anything.
func TestSharedScope_ClonesPerWaiter(t *testing.T) {
	files := []gitdomain.ReviewFileSummary{
		{Path: "a.go", Status: gitdomain.GitFileStatusModified, Additions: 3, Deletions: 1},
	}
	first, err := branchreview.SharedScope("base-sha", files)
	require.NoError(t, err)
	second, err := branchreview.SharedScope("base-sha", files)
	require.NoError(t, err)

	first.Files[0].Additions = 9999

	assert.Equal(t, 3, second.Files[0].Additions, "waiters must not share one backing array")
	assert.Equal(t, 3, files[0].Additions, "the flight's own slice must not be mutable through a waiter")
	assert.Equal(t, "base-sha", second.Base)
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
