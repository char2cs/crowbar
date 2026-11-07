package worktreepath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

// Reach makes every directory from the home down to a workspace root
// traversable by group and others, adding only the search bits; Unreachable
// names the ones it would change.
func TestReach_AddsTraverseFromTheHomeDownToTheWorkspaceRoot(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	home := filepath.Join(t.TempDir(), "crowbar")
	wt := filepath.Join(home, "projects", "p1", "acme", "main", "worktree")
	require.NoError(t, os.MkdirAll(wt, 0o700))
	require.NoError(t, os.Chmod(filepath.Join(home, "projects", "p1"), 0o750))
	require.NoError(t, os.Chmod(wt, 0o700))

	missing, err := Unreachable(home, wt)
	require.NoError(t, err)
	assert.Equal(t, []string{".", "projects", "projects/p1", "projects/p1/acme", "projects/p1/acme/main"}, missing)
	require.NoError(t, Reach(home, wt))

	for dir, want := range map[string]os.FileMode{
		"": 0o711, "projects": 0o711, "projects/p1": 0o751, "projects/p1/acme": 0o711,
		"projects/p1/acme/main": 0o711, "projects/p1/acme/main/worktree": 0o700,
	} {
		info, err := os.Stat(filepath.Join(home, dir))
		require.NoError(t, err)
		assert.Equal(t, want, info.Mode().Perm(), dir)
	}
	missing, err = Unreachable(home, wt)
	require.NoError(t, err)
	assert.Empty(t, missing)
	require.Error(t, Reach(home, filepath.Join(t.TempDir(), "worktree")), "a path outside the home")
}

// HomeOf finds the Crowbar home a managed checkout lives in by its LAYOUT file.
func TestHomeOf_FindsTheHomeAboveAManagedCheckout(t *testing.T) {
	home := filepath.Join(t.TempDir(), "projects", "crowbar")
	wt := filepath.Join(home, "projects", "p1", "projects", "main", "worktree")
	require.NoError(t, os.MkdirAll(wt, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "LAYOUT"), []byte("1\n"), 0o600))

	got, ok := HomeOf(wt)
	assert.True(t, ok)
	assert.Equal(t, home, got)
	_, ok = HomeOf(filepath.Join(t.TempDir(), "x", "worktree"))
	assert.False(t, ok, "no home above")
}
