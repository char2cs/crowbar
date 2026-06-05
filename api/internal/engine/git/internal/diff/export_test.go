package diff

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ParseFilesFromText is an export shim for benchmarks and white-box tests.
func ParseFilesFromText(ctx context.Context, repoPath string, text string) []domain.FileDiff {
	return parseFiles(ctx, repoPath, text)
}
