package git_test

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"

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

// waitUntilParkedOnRLock blocks until a goroutine running WouldMergeConflict is
// parked inside sync.(*RWMutex).RLock.
//
// This is the real signal that the read is *waiting for the lock*: the runtime's
// own goroutine dump says so. A duration cannot say so — sleeping 50ms and
// finding the read unfinished proves nothing about why, and under load it is
// simply a guess. The wait is bounded by the reader itself, so it always ends in
// one of three states, never in a timeout: the read parks (return), the read
// reaches its git subprocess, or the read completes. The last two are exactly the
// regression this test exists to catch, and both fail immediately.
func waitUntilParkedOnRLock(
	t *testing.T,
	readDone <-chan struct{},
	reachedExec <-chan string,
) {
	t.Helper()
	buf := make([]byte, 1<<16)
	for {
		select {
		case <-readDone:
			t.Fatal("read completed while a write held the exclusive lock")
		case op := <-reachedExec:
			t.Fatalf("read reached its git subprocess (%s) while a write held the lock", op)
		default:
		}

		n := runtime.Stack(buf, true)
		for n == len(buf) {
			buf = make([]byte, 2*len(buf))
			n = runtime.Stack(buf, true)
		}
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(stack, "sync.(*RWMutex).RLock") &&
				strings.Contains(stack, "WouldMergeConflict") {
				return
			}
		}
		runtime.Gosched()
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
	for range 2 {
		go func() {
			defer wg.Done()
			_, _ = e.WouldMergeConflict(ctx, dir, "a", "b")
		}()
	}

	// Both reads must reach their blocking exec call — proving neither waited for
	// the other to finish (both hold RLock concurrently). The receives ARE the
	// assertion: the first read never returns (it is parked on release), so if
	// reads serialized, the second receive could never complete and `go test
	// -timeout` reports it with a full goroutine dump. No wall-clock guess needed.
	<-started
	<-started

	close(release)
	wg.Wait()
}

func TestRWMutex_WriteBlocksConcurrentRead(t *testing.T) {
	dir := t.TempDir()
	started := make(chan string, 2)
	release := make(chan struct{})
	e := git.NewWithExec(blockingExec(started, release))
	ctx := context.Background()

	writeDone := make(chan struct{})
	go func() {
		_ = e.Commit(ctx, dir, "subject", "")
		close(writeDone)
	}()

	// The write is now inside its exec call, holding the exclusive lock. Every
	// later value on started can therefore only come from the read.
	<-started

	readDone := make(chan struct{})
	go func() {
		_, _ = e.WouldMergeConflict(ctx, dir, "a", "b")
		close(readDone)
	}()

	// Blocks until the runtime reports the read parked on the lock — and fails the
	// instant the read instead slips through to its git subprocess or finishes.
	waitUntilParkedOnRLock(t, readDone, started)

	close(release)
	<-writeDone

	// Released: the read must now proceed. A read that stays parked hangs here and
	// is reported by `go test -timeout`.
	<-readDone
}

func TestRWMutex_DiscardDoesNotDeadlockOnStatus(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	require.NoError(t, osWriteFile(dir, "file.txt", "changed\n"))

	e := git.New()
	done := make(chan error, 1)
	go func() { done <- e.Discard(context.Background(), dir, "file.txt") }()

	// If Status re-took the lock Discard already holds, this receive never
	// completes and `go test -timeout` dumps the deadlocked goroutines — strictly
	// more diagnostic than a 2s t.Fatal, and it cannot false-fail under load.
	require.NoError(t, <-done)
}
