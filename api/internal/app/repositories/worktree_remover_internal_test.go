package repositories

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorktreeRemover_RemovesWorkspaceRootIncludingSiblingChatsDir pins the
// workspace-root split (spec §3.5): the async delete reactor's rm -rf must
// remove the WHOLE workspace root — the "worktree" leaf `git worktree remove`
// already cleared plus the sibling "chats" tree it never touches — not just
// the worktree leaf itself. Without this, every agentic chat's ledger and
// segment tmp dirs would survive a workspace delete as an orphaned directory.
func TestWorktreeRemover_RemovesWorkspaceRootIncludingSiblingChatsDir(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "feat-x")
	worktreeLeaf := filepath.Join(root, "worktree")
	chatsDir := filepath.Join(root, "chats", "chatA", "ledger")

	// git worktree remove already cleared the "worktree" leaf by the time the
	// reactor's rm runs; only the sibling "chats" tree remains on disk.
	require.NoError(t, os.MkdirAll(chatsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chatsDir, "00000001.turn"), []byte("{}"), 0o644))

	remove := worktreeRemover(home)
	require.NoError(t, remove(worktreeLeaf))

	_, err := os.Stat(root)
	assert.True(t, os.IsNotExist(err), "the whole workspace root (worktree + sibling chats) must be gone")
}

// TestWorktreeRemover_RefusesPathOutsideCrowbarHome guards the pre-existing
// safety invariant: an adopted home/main worktree's mapped path (outside the
// crowbar home) is the user's REAL checkout and must never be rm -rf'd, even
// after switching the removal target to the parent directory.
func TestWorktreeRemover_RefusesPathOutsideCrowbarHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir() // a sibling temp dir, NOT under home
	userRepo := filepath.Join(outside, "user-repo")
	require.NoError(t, os.MkdirAll(userRepo, 0o755))

	remove := worktreeRemover(home)
	require.NoError(t, remove(userRepo))

	_, err := os.Stat(userRepo)
	assert.NoError(t, err, "a path outside the crowbar home must never be removed")
}

// TestWorktreeRemover_HomeKindWorkspace_NeverRemovesRepoPath is the explicit
// home-kind safety proof (Task 7): a home-kind workspace's WorktreePath is the
// user's REAL repository (repo.Path, OUTSIDE crowbar home). The delete cascade
// must never rm that path OR its parent — doing so would delete the user's actual
// repo (and its siblings) off their disk. This pins the outside-home guard against
// a real repo.Path shape (with a .git dir and a sibling checkout), not a bare temp
// dir, so a regression that reroutes the rm can't slip past.
func TestWorktreeRemover_HomeKindWorkspace_NeverRemovesRepoPath(t *testing.T) {
	home := t.TempDir()
	userRoot := t.TempDir()                        // stands in for ~/code, OUTSIDE home
	repoPath := filepath.Join(userRoot, "my-repo") // the home-kind ws's WorktreePath
	sibling := filepath.Join(userRoot, "other-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	remove := worktreeRemover(home)
	require.NoError(t, remove(repoPath))

	_, err := os.Stat(repoPath)
	assert.NoError(t, err, "a home-kind workspace's repo.Path must never be removed")
	_, err = os.Stat(sibling)
	assert.NoError(t, err, "repo.Path's PARENT (and its siblings) must never be touched")
}

// TestWorktreeRemover_RefusesDegenerateLeafDirectlyUnderHome hardens the guard
// (Task 7): a path that is a direct child of home (so filepath.Dir(path) == home)
// must NOT cause the rm to delete crowbar home ITSELF. The removal target is the
// PARENT of the worktree leaf, so a one-segment path is refused — only a path with
// an intermediate segment below home has a parent still strictly under home.
func TestWorktreeRemover_RefusesDegenerateLeafDirectlyUnderHome(t *testing.T) {
	home := t.TempDir()
	leaf := filepath.Join(home, "worktree") // filepath.Dir(leaf) == home
	require.NoError(t, os.MkdirAll(leaf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "sentinel"), []byte("x"), 0o644))

	remove := worktreeRemover(home)
	require.NoError(t, remove(leaf))

	_, err := os.Stat(home)
	assert.NoError(t, err, "crowbar home itself must never be removed by a degenerate leaf")
	_, err = os.Stat(filepath.Join(home, "sentinel"))
	assert.NoError(t, err, "home's other contents must survive")
}
