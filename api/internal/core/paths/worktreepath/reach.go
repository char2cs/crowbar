package worktreepath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// traverse is the search bit for group and others: enough to reach a known path
// below a directory, never to list it.
const traverse fs.FileMode = 0o011

// Reach gives group and others traverse on every directory from home down to
// the workspace root holding worktreePath, adding only that bit, so another uid
// (an IDE, a container) reaches the checkout as git made it.
func Reach(home, worktreePath string) error {
	return walkChain(home, worktreePath, func(root *os.Root, rel string, perm fs.FileMode) error {
		return root.Chmod(rel, perm|traverse)
	})
}

// Unreachable names, relative to home, the directories Reach would change.
func Unreachable(home, worktreePath string) ([]string, error) {
	var out []string
	err := walkChain(home, worktreePath, func(_ *os.Root, rel string, _ fs.FileMode) error {
		out = append(out, rel)
		return nil
	})
	return out, err
}

// walkChain hands fix each directory from home down to worktreePath's parent
// that lacks traverse. It works through an os.Root, so a symlink cannot lead it
// out of the home.
func walkChain(home, worktreePath string, fix func(*os.Root, string, fs.FileMode) error) (err error) {
	rel, err := filepath.Rel(filepath.Join(home, "projects"), filepath.Dir(worktreePath))
	if err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("worktreepath: reach %q: not under %q", worktreePath, filepath.Join(home, "projects"))
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return fmt.Errorf("worktreepath: reach: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	chain := []string{".", "projects"}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		chain = append(chain, filepath.Join(chain[len(chain)-1], part))
	}
	for _, dir := range chain {
		info, err := root.Lstat(dir)
		if err != nil {
			return fmt.Errorf("worktreepath: reach: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("worktreepath: reach: %s is not a directory", dir)
		}
		if perm := info.Mode().Perm(); perm&traverse != traverse {
			if err := fix(root, dir, perm); err != nil {
				return fmt.Errorf("worktreepath: reach: %w", err)
			}
		}
	}
	return nil
}

// HomeOf finds the Crowbar home holding a managed checkout: the nearest
// directory above it with a LAYOUT file, which only a home has.
func HomeOf(worktreePath string) (string, bool) {
	for dir := filepath.Dir(worktreePath); ; dir = filepath.Dir(dir) {
		if info, err := os.Lstat(filepath.Join(dir, "LAYOUT")); err == nil && info.Mode().IsRegular() {
			rel, err := filepath.Rel(filepath.Join(dir, "projects"), worktreePath)
			return dir, err == nil && filepath.IsLocal(rel)
		}
		if filepath.Dir(dir) == dir {
			return "", false
		}
	}
}
