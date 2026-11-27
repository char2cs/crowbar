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
			_ = e.WouldMergeConflicts(ctx, dir, []git.MergePair{{Ours: "a", Theirs: "b"}})
		}()
	}

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

	<-started

	readDone := make(chan struct{})
	go func() {
		_ = e.WouldMergeConflicts(ctx, dir, []git.MergePair{{Ours: "a", Theirs: "b"}})
		close(readDone)
	}()

	waitUntilParkedOnRLock(t, readDone, started)

	close(release)
	<-writeDone

	<-readDone
}

func TestRWMutex_DiscardDoesNotDeadlockOnStatus(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	require.NoError(t, osWriteFile(dir, "file.txt", "changed\n"))

	e := git.New()
	done := make(chan error, 1)
	go func() { done <- e.Discard(context.Background(), dir, "file.txt") }()

	require.NoError(t, <-done)
}
