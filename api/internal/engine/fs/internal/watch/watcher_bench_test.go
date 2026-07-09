package watch_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

// benchGitProvider is a zero-overhead stub for BenchmarkFanOut.
type benchGitProvider struct{}

func (p *benchGitProvider) ComputeStatus(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{Branch: "main"}, nil
}

func (p *benchGitProvider) ComputeWorkingTreeSummary(
	_ context.Context,
	_ string,
	_ string,
) (int, int, bool, bool, error) {
	return 0, 0, false, false, nil
}

// benchDispatcher is a no-op Dispatcher for benchmarks.
type benchDispatcher struct{}

func (d *benchDispatcher) OnFileChange(_ context.Context, _ domain.FileChangeEvent)       {}
func (d *benchDispatcher) OnGitStatus(_ context.Context, _ string, _ gitdomain.GitStatus) {}
func (d *benchDispatcher) OnSyncWorkingTreeState(_ context.Context, _ watch.SyncInput)    {}

func BenchmarkFanOut(b *testing.B) {
	dir := b.TempDir()

	// init a minimal git repo so git check-ignore and ComputeStatus work.
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "bench@bench.com"},
		{"config", "user.name", "Bench"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Run()
	}

	provider := &benchGitProvider{}
	dispatcher := &benchDispatcher{}
	w := watch.NewWatcher("bench-ws", dir, "", provider, dispatcher)
	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		b.Skip("watcher start failed:", err)
	}
	defer w.Stop()

	// Prime the watcher so OS-level registration is complete.
	benchFile := filepath.Join(dir, "bench.txt")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		os.WriteFile(benchFile, []byte("x"), 0o600)
	}
}
