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

func workspacesByID() *mockWorkspace {
	return &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{
				ID:           id,
				RepoID:       "repo1",
				Branch:       "feature",
				WorktreePath: "/wt/" + id,
				ForkPointSha: "fork-" + id,
				Provisioning: domain.WorkspaceProvisioned,
			}, nil
		},
	}
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

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

func TestGetFiles_SingleFlightsConcurrentCallers(t *testing.T) {
	const callers = 8

	git := newBlockingGit(callers)
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
	require.ErrorIs(t, <-leaderErr, context.Canceled,
		"a caller must still honour its own cancellation")
	assert.NoError(t, git.ctxSeen.Err(),
		"the shared computation must survive the cancellation of the caller that started it")

	close(git.release)
	assert.Equal(t, int64(1), git.invocations.Load())
}

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
