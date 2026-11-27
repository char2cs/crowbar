package diff

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

type GitRunner func(ctx context.Context, dir string, args ...string) exec.Result

func SetGitRunner(fn GitRunner) (GitRunner, func()) {
	prev := runGit
	runGit = fn
	return prev, func() { runGit = prev }
}

func ParseFilesFromText(ctx context.Context, repoPath, text string) []gitdomain.FileDiff {
	return parseFiles(ctx, repoPath, text)
}

func SetPathspecChunkBytes(n int) func() {
	prev := pathspecChunkBytes
	pathspecChunkBytes = n
	return func() { pathspecChunkBytes = prev }
}

func ResetSummaryCache() {
	committedCounts.reset()
}

func SummaryCacheSize() int {
	return committedCounts.size()
}

const SummaryCacheCap = summaryCacheEntries

func (c *summaryCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *summaryCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
	c.order = nil
}
