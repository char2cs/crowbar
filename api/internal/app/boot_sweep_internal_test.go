package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
)

// fakeWorkspacePaths is a minimal wspaths.WorkspacePaths double: Get returns a
// preset path, Delete records that it was called. It lets bootSweepPurge run its
// fs teardown without a real view.db.
type fakeWorkspacePaths struct {
	path    string
	deleted bool
}

func (f *fakeWorkspacePaths) Put(_ context.Context, _ string, _ string) error { return nil }

func (f *fakeWorkspacePaths) Get(_ context.Context, _ string) (string, error) {
	return f.path, nil
}

func (f *fakeWorkspacePaths) Delete(_ context.Context, _ string) error {
	f.deleted = true
	return nil
}

var _ wspaths.WorkspacePaths = (*fakeWorkspacePaths)(nil)

// TestBootSweepPurge_RemovesWorkspaceRootIncludingSiblingChats is the parallel
// twin of repositories.TestWorktreeRemover_RemovesWorkspaceRootIncludingSiblingChatsDir:
// both the async delete reactor (worktreeRemover) and the crash-recovery boot
// orphan-sweep (bootSweepPurge) must rm the WHOLE workspace root — the "worktree"
// leaf plus the sibling "chats" tree — via filepath.Dir(path), not just the
// worktree leaf. This guards bootSweepPurge's parent-removal so a future edit to
// one site can't silently regress the other (spec §3.5/§3.8). The Forget of a
// non-existent aggregate returns ErrValidation, which bootSweepPurge swallows, so
// the fs teardown is exercised without needing a persisted workspace.
func TestBootSweepPurge_RemovesWorkspaceRootIncludingSiblingChats(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	root := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "feat-x")
	worktreeLeaf := filepath.Join(root, "worktree")
	chatsDir := filepath.Join(root, "chats", "chatA", "ledger")

	// The "worktree" leaf was already cleared by git; only the sibling chats tree
	// remains on disk when the boot sweep re-drives the purge.
	require.NoError(t, os.MkdirAll(chatsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chatsDir, "00000001.turn"), []byte("{}"), 0o644))

	paths := &fakeWorkspacePaths{path: worktreeLeaf}
	purge := bootSweepPurge(c.axWorkspace, paths, home)
	require.NoError(t, purge(context.Background(), "ws-does-not-exist"))

	_, err := os.Stat(root)
	assert.True(t, os.IsNotExist(err),
		"boot sweep must remove the whole workspace root (worktree + sibling chats)")
	assert.True(t, paths.deleted, "boot sweep must delete the id-path row")
}

// TestBootSweepPurge_RefusesPathOutsideCrowbarHome guards the safety invariant
// (mirrors worktreeRemover's guard test): an adopted home/main worktree's mapped
// path is the user's REAL checkout outside the crowbar home and must never be
// rm'd, even after switching the removal target to the parent directory.
func TestBootSweepPurge_RefusesPathOutsideCrowbarHome(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	outside := t.TempDir() // NOT under home
	userRepo := filepath.Join(outside, "user-repo")
	require.NoError(t, os.MkdirAll(userRepo, 0o755))

	paths := &fakeWorkspacePaths{path: userRepo}
	purge := bootSweepPurge(c.axWorkspace, paths, home)
	require.NoError(t, purge(context.Background(), "ws-x"))

	_, err := os.Stat(userRepo)
	assert.NoError(t, err, "a path outside the crowbar home must never be removed")
}

// TestBootSweepPurge_HomeKindWorkspace_NeverRemovesRepoPath is the boot-sweep twin
// of the delete reactor's home-kind safety proof (Task 7): a crash-recovery
// re-drive of a tombstoned home-kind workspace must never rm the user's REAL
// repo.Path (outside crowbar home) or its parent.
func TestBootSweepPurge_HomeKindWorkspace_NeverRemovesRepoPath(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	userRoot := t.TempDir()
	repoPath := filepath.Join(userRoot, "my-repo")
	sibling := filepath.Join(userRoot, "other-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	paths := &fakeWorkspacePaths{path: repoPath}
	purge := bootSweepPurge(c.axWorkspace, paths, home)
	require.NoError(t, purge(context.Background(), "ws-home"))

	_, err := os.Stat(repoPath)
	assert.NoError(t, err, "a home-kind workspace's repo.Path must never be removed by the boot sweep")
	_, err = os.Stat(sibling)
	assert.NoError(t, err, "repo.Path's PARENT (and siblings) must never be touched")
}

// TestBootSweepPurge_RefusesDegenerateLeafDirectlyUnderHome hardens the boot-sweep
// guard (Task 7): a residual path that is a direct child of home (filepath.Dir ==
// home) must never make the sweep rm crowbar home itself.
func TestBootSweepPurge_RefusesDegenerateLeafDirectlyUnderHome(t *testing.T) {
	c := newSweepContainer(t)
	home := t.TempDir()
	leaf := filepath.Join(home, "worktree") // filepath.Dir(leaf) == home
	require.NoError(t, os.MkdirAll(leaf, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "sentinel"), []byte("x"), 0o644))

	paths := &fakeWorkspacePaths{path: leaf}
	purge := bootSweepPurge(c.axWorkspace, paths, home)
	require.NoError(t, purge(context.Background(), "ws-leaf"))

	_, err := os.Stat(home)
	assert.NoError(t, err, "crowbar home itself must never be removed by a degenerate leaf")
	_, err = os.Stat(filepath.Join(home, "sentinel"))
	assert.NoError(t, err, "home's other contents must survive")
}
