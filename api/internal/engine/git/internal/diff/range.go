package diff

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// Range returns the three-dot diff between base and branch (09 §2).
// It runs `git diff -M <base>...<branch>` and returns a MultiFileDiff
// with Files, TotalFiles, TotalAdditions, and TotalDeletions populated.
// Commit metadata fields are always zero-value.
func Range(
	ctx context.Context,
	repoPath string,
	base string,
	branch string,
) (gitdomain.MultiFileDiff, error) {
	r := exec.Git(ctx, repoPath, "diff", "-M", base+"..."+branch)
	if err := exec.RequireSuccess("diff: range", r); err != nil {
		return gitdomain.MultiFileDiff{}, err
	}
	files := parseFiles(ctx, repoPath, r.Stdout)
	totalAdd, totalDel := totals(files)
	return gitdomain.MultiFileDiff{
		Files:          files,
		TotalFiles:     len(files),
		TotalAdditions: totalAdd,
		TotalDeletions: totalDel,
	}, nil
}
