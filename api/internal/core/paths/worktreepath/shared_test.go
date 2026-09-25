package worktreepath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsLiveCheckout_IsAGitEntry(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, IsLiveCheckout(dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: x"), 0o600))
	assert.True(t, IsLiveCheckout(dir), "a linked worktree's .git is a file")
}

func TestSiblingRoots_ListsTheSlugDirectory(t *testing.T) {
	home := t.TempDir()
	roots, err := SiblingRoots(home, "p1", "github.com/acme/app")
	require.NoError(t, err)
	assert.Empty(t, roots, "a slug not created yet has no siblings")

	slug := filepath.Join(home, "projects", "p1", "github.com", "acme", "app")
	require.NoError(t, os.MkdirAll(filepath.Join(slug, "main"), 0o755))
	roots, err = SiblingRoots(home, "p1", "github.com/acme/app")
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(slug, "main")}, roots)
}

func TestSamePath_ResolvesSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(real, link))
	assert.True(t, SamePath(real, link))
	assert.False(t, SamePath(real, t.TempDir()))
}
