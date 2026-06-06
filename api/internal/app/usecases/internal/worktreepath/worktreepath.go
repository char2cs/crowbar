// Package worktreepath derives a deterministic filesystem path for a
// git worktree from a repository path and a branch name. All unsafe
// characters in the branch name are replaced with hyphens so the
// resulting directory is safe on every major OS. A short FNV-1a hash
// of the original (unsanitised) branch name is appended to guarantee
// the mapping is injective: two branches whose names differ only in
// unsafe characters (e.g. "feature/foo" vs "feature-foo") are always
// assigned distinct directories.
package worktreepath

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
)

// unsafeChars contains every character that is problematic in directory
// names across Linux, macOS, and Windows.
const unsafeChars = `/\:*?"<>|@#`

// For returns the deterministic worktree directory path for the given
// repository path and branch name. The directory is rooted at:
//
//	<repoParent>/.crowbar-worktrees/<repoBase>/<sanitized-branch>-<8hexFNV(branch)>
//
// Branch name sanitisation replaces every character in unsafeChars with a
// hyphen. The eight-character hex FNV-1a suffix makes the mapping injective:
// branches that are identical after sanitisation (e.g. "feature/foo" and
// "feature-foo") still receive distinct directories. Same inputs always
// produce the same output.
func For(
	repoPath string,
	branch string,
) string {
	repoParent := filepath.Dir(repoPath)
	repoBase := filepath.Base(repoPath)
	dir := sanitize(branch) + "-" + hashBranch(branch)
	return filepath.Join(repoParent, ".crowbar-worktrees", repoBase, dir)
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

// hashBranch returns an 8-character lowercase hex FNV-1a digest of branch.
func hashBranch(
	branch string,
) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(branch))
	return fmt.Sprintf("%08x", h.Sum32())
}
