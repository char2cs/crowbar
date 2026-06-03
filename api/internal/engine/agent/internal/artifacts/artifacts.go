// Package artifacts captures git diffs from agent worktrees.
package artifacts

import (
	"fmt"
	"os"
	"os/exec"
)

// WriteDiff runs `git diff {baseBranch}...{branchName}` in worktreePath
// and writes the output to destPath. An empty diff is valid and results in
// an empty file.
func WriteDiff(
	worktreePath string,
	baseBranch string,
	branchName string,
	destPath string,
) error {
	ref := fmt.Sprintf("%s...%s", baseBranch, branchName)
	cmd := exec.Command("git", "diff", ref)
	cmd.Dir = worktreePath

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("artifacts: git diff %s: %w", ref, err)
	}

	if err := os.WriteFile(destPath, out, 0o644); err != nil {
		return fmt.Errorf("artifacts: write %s: %w", destPath, err)
	}
	return nil
}
