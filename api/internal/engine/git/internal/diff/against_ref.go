package diff

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// AgainstRef returns the diff of the working tree against ref, including both
// committed changes since ref and uncommitted tracked modifications. It runs
// `git diff -M <ref> --` (two-dot-to-working-tree) and returns a MultiFileDiff.
// Commit metadata fields are always zero-value. Untracked files are not
// included (they have no diff against the index); they surface via git status.
func AgainstRef(
	ctx context.Context,
	repoPath string,
	ref string,
) (gitdomain.MultiFileDiff, error) {
	r := exec.Git(ctx, repoPath, "diff", "-M", ref, "--")
	if err := exec.RequireSuccess("diff: against ref", r); err != nil {
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
