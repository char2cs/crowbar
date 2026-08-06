package repositories

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The remover deletes the PARENT of the path it is given, because a workspace's
// worktree sits in a root beside its chats tree. That parent is only the right
// thing to delete when the path really is an identity-keyed worktree.
//
// A pre-leaf workspace recorded at <slug>/<branch> has <slug> as its parent —
// the directory holding EVERY branch of that repo. Under a prefix-only "is it
// under the home" guard, removing one such workspace would take all of them.
func TestWorktreeRemover_RefusesAPathThatIsNotAWorkspaceWorktree(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "P", "github.com/acme/app")
	victim := filepath.Join(slug, "develop")           // the pre-leaf workspace
	sibling := filepath.Join(slug, "main", "worktree") // another branch entirely
	require.NoError(t, os.MkdirAll(victim, 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	require.NoError(t, worktreeRemover(home)(victim))

	assert.DirExists(t, sibling, "removing one workspace must never take its siblings")
	assert.DirExists(t, slug, "the slug directory is not a workspace root")
	assert.DirExists(t, victim, "and the refusal leaves the workspace itself alone")
}

// The identity-keyed shape is removed, root and all.
func TestWorktreeRemover_RemovesAnIdentityKeyedRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "projects", "P", "workspaces", "W1")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats"), 0o755))

	require.NoError(t, worktreeRemover(home)(filepath.Join(root, "worktree")))

	assert.NoDirExists(t, root, "the whole workspace root is the footprint")
}
