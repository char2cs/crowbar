package diff

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ParseFilesFromText is an export shim for benchmarks and white-box tests.
func ParseFilesFromText(ctx context.Context, repoPath string, text string) []gitdomain.FileDiff {
	return parseFiles(ctx, repoPath, text)
}
