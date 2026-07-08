package git_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// blockingExec returns an execFn-compatible function that answers
// "rev-parse --git-common-dir" immediately (so common-dir resolution, which
// runs before any lock is taken, never blocks) and otherwise signals on
// started before blocking on release — letting a test control exactly when a
// git subprocess "returns" without sleeping.
func blockingExec(
	started chan<- string,
	release <-chan struct{},
) func(ctx context.Context, dir string, args ...string) gitexec.Result {
	return func(_ context.Context, dir string, args ...string) gitexec.Result {
		if len(args) > 0 && args[0] == "rev-parse" {
			return gitexec.Result{ExitCode: 0, Stdout: dir}
		}
		started <- args[0]
		<-release
		return gitexec.Result{ExitCode: 0}
	}
}

func TestRWMutex_ConcurrentReadsDoNotSerialize(t *testing.T) {
	dir := t.TempDir()
	started := make(chan string, 2)
	release := make(chan struct{})
	e := git.NewWithExec(blockingExec(started, release))
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, _ = e.WouldMergeConflict(ctx, dir, "a", "b")
		}()
	}

	// Both reads must reach their blocking exec call — proving neither waited
	// for the other to finish (both hold RLock concurrently).
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("both concurrent reads did not start within 1s — a read is blocking on another read's RLock")
		}
	}
	close(release)
	wg.Wait()
}

func TestRWMutex_WriteBlocksConcurrentRead(t *testing.T) {
	dir := t.TempDir()
	started := make(chan string, 1)
	release := make(chan struct{})
	e := git.NewWithExec(blockingExec(started, release))
	ctx := context.Background()

	writeDone := make(chan struct{})
	go func() {
		_ = e.Commit(ctx, dir, "subject", "")
		close(writeDone)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("write never reached its exec call")
	}

	readDone := make(chan struct{})
	go func() {
		_, _ = e.WouldMergeConflict(ctx, dir, "a", "b")
		close(readDone)
	}()

	// The read must NOT complete while the write still holds the lock.
	select {
	case <-readDone:
		t.Fatal("read completed while a write held the lock")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-writeDone
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("read did not proceed after the write released the lock")
	}
}

func TestRWMutex_DiscardDoesNotDeadlockOnStatus(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	require.NoError(t, osWriteFile(dir, "file.txt", "changed\n"))

	e := git.New()
	done := make(chan error, 1)
	go func() { done <- e.Discard(context.Background(), dir, "file.txt") }()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Discard deadlocked (Status must not re-take the lock Discard already holds)")
	}
}
