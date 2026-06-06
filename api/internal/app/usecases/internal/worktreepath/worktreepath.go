// Package worktreepath derives a deterministic filesystem path for a
// git worktree from a repository path and a branch name. All unsafe
// characters in the branch name are replaced with hyphens so the
// resulting directory is safe on every major OS.
package worktreepath

import (
	"path/filepath"
	"strings"
)

// unsafeChars contains every character that is problematic in directory
// names across Linux, macOS, and Windows.
const unsafeChars = `/\:*?"<>|@#`

// For returns the deterministic worktree directory path for the given
// repository path and branch name. The directory is rooted at:
//
//	<repoParent>/.crowbar-worktrees/<repoBase>/<sanitized-branch>
//
// Branch name sanitisation replaces every character in unsafeChars with a
// hyphen. Same inputs always produce the same output; different branches
// always produce different outputs.
func For(
	repoPath string,
	branch string,
) string {
	repoParent := filepath.Dir(repoPath)
	repoBase := filepath.Base(repoPath)
	sanitized := sanitize(branch)
	return filepath.Join(repoParent, ".crowbar-worktrees", repoBase, sanitized)
}

func sanitize(
	branch string,
) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(unsafeChars, r) {
			return '-'
		}
		return r
	}, branch)
}
