package diff

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// AgainstRef returns `git diff -M <ref> --`: committed changes since ref plus uncommitted
// tracked modifications, with zero commit metadata. Untracked files surface via status.
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
