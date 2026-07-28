package diff

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// GitRunner is the signature of the seam the committed-count query reaches git
// through.
type GitRunner func(ctx context.Context, dir string, args ...string) exec.Result

// SetGitRunner replaces that seam so a test can count invocations of the cached
// committed-count query. A cache hit is asserted by that count, never by
// timing. It returns a restore function and the runner it replaced, so a test
// can wrap the real one instead of stubbing it out.
func SetGitRunner(fn GitRunner) (GitRunner, func()) {
	prev := runGit
	runGit = fn
	return prev, func() { runGit = prev }
}

// ParseFilesFromText is an export shim for benchmarks and white-box tests.
func ParseFilesFromText(ctx context.Context, repoPath, text string) []gitdomain.FileDiff {
	return parseFiles(ctx, repoPath, text)
}

// SetPathspecChunkBytes shrinks the per-invocation pathspec budget so a test can
// force the dirty-count query across several git invocations without building an
// ARG_MAX-sized repo. It returns a restore function.
func SetPathspecChunkBytes(n int) func() {
	prev := pathspecChunkBytes
	pathspecChunkBytes = n
	return func() { pathspecChunkBytes = prev }
}

// ResetSummaryCache drops every cached committed-count entry so a test starts
// from a known-cold cache.
func ResetSummaryCache() {
	committedCounts.reset()
}

// SummaryCacheSize reports how many committed-count entries are held.
func SummaryCacheSize() int {
	return committedCounts.size()
}

// SummaryCacheCap is the entry cap eviction enforces.
const SummaryCacheCap = summaryCacheEntries
