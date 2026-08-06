package worktreepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The navigable alias tree: symlinks at <home>/projects/<project>/<slug>/<branch>
// pointing at the identity-keyed workspace root.
//
// A workspace's real location is its UUID (WorkspaceRootByID). The alias is what
// makes that liveable: it is the path a shell prompt, an editor title bar and an
// agent cwd show, and — because it sits exactly where the old real directories
// sat — every absolute path recorded before this layout existed still resolves
// through it without being rewritten.
//
// Aliases are DERIVED state. They are rebuilt from the current slug and branch
// whenever either changes, which is why a rename is a symlink swap rather than a
// directory move, and why a slug that resolves differently later is cosmetic
// rather than a workspace stranded in a directory nobody looks in again.

// LinkAlias points alias at root, replacing whatever is already there.
//
// Replacement is unconditional but narrow: only a symlink is ever removed to
// make room. A real directory at the alias path is a workspace from before this
// layout that has not been migrated, or a collision, and either way silently
// deleting it would destroy a worktree — so it is refused.
func LinkAlias(
	alias string,
	root string,
) error {
	if alias == "" || root == "" {
		return fmt.Errorf("worktreepath: link alias requires non-empty alias and root")
	}
	// An ANCESTOR of the new alias may itself be an alias: renaming testing to
	// testing/x needs <slug>/testing to become a directory, and it is currently a
	// symlink. MkdirAll would happily walk THROUGH it and plant the new link
	// inside the workspace root it points at.
	//
	// Clearing it is safe and is not a special case: git's own ref rules forbid
	// `testing` and `testing/x` existing at once, so an ancestor alias is by
	// definition a name no live branch holds any more. The walk stops at the
	// first real directory, which is the slug dir.
	if err := clearAliasAncestors(filepath.Dir(alias)); err != nil {
		return err
	}
	//nolint:gosec // G301: the alias tree mirrors the worktree layout's own 0755.
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		return fmt.Errorf("worktreepath: create alias parent: %w", err)
	}
	switch info, err := os.Lstat(alias); {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		if rmErr := os.Remove(alias); rmErr != nil {
			return fmt.Errorf("worktreepath: replace alias %q: %w", alias, rmErr)
		}
	case err == nil:
		return fmt.Errorf("worktreepath: alias %q is a real directory, not a link", alias)
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("worktreepath: inspect alias %q: %w", alias, err)
	}
	if err := os.Symlink(root, alias); err != nil {
		return fmt.Errorf("worktreepath: link alias %q: %w", alias, err)
	}
	return nil
}

// UnlinkAlias removes a workspace's alias and any parent directories the removal
// emptied, stopping at floor (the repo's slug directory).
//
// Only a SYMLINK is unlinked — a real directory at the alias path is never
// touched, for the same reason LinkAlias refuses to replace one. The parent walk
// uses os.Remove, which only ever succeeds on an empty directory, so a nested
// alias parent another branch still occupies stops it.
func UnlinkAlias(
	alias string,
	floor string,
) error {
	if alias == "" {
		return nil
	}
	info, err := os.Lstat(alias)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("worktreepath: inspect alias %q: %w", alias, err)
	case info.Mode()&os.ModeSymlink == 0:
		return fmt.Errorf("worktreepath: alias %q is a real directory, not a link", alias)
	}
	if rmErr := os.Remove(alias); rmErr != nil {
		return fmt.Errorf("worktreepath: unlink alias %q: %w", alias, rmErr)
	}
	for dir := filepath.Dir(alias); UnderHome(dir, floor); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			return nil
		}
	}
	return nil
}

// ResolveAlias reports the root an alias points at, and false when the path is
// not a symlink (missing, or a not-yet-migrated real directory).
func ResolveAlias(alias string) (string, bool) {
	info, err := os.Lstat(alias)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	target, err := os.Readlink(alias)
	if err != nil {
		return "", false
	}
	return target, true
}

// clearAliasAncestors removes symlinks standing where the new alias needs real
// directories, deepest first. A real directory (or a missing one) ends the walk.
func clearAliasAncestors(dir string) error {
	var links []string
	for d := dir; ; d = filepath.Dir(d) {
		info, err := os.Lstat(d)
		if err != nil {
			if filepath.Dir(d) == d {
				break
			}
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			break
		}
		links = append(links, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	for _, l := range links {
		if err := os.Remove(l); err != nil {
			return fmt.Errorf("worktreepath: clear alias ancestor %q: %w", l, err)
		}
	}
	return nil
}
