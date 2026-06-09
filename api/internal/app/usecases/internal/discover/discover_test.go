package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTree creates the given directories under root and returns root.
func buildTree(
	t *testing.T,
	dirs []string,
) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	return root
}

func TestRepos_FindsGitDirsAtOrWithinMaxDepth(t *testing.T) {
	// depth=1 repo at root/a/.git
	// depth=2 repo at root/x/b/.git
	// depth=3 repo at root/x/y/c/.git — beyond maxDepth=2, should be skipped
	root := buildTree(t, []string{
		"a/.git",
		"x/b/.git",
		"x/y/c/.git",
	})

	got, err := Repos(root, 2)
	require.NoError(t, err)

	wantA := filepath.Join(root, "a")
	wantB := filepath.Join(root, "x", "b")
	assert.ElementsMatch(t, []string{wantA, wantB}, got)
}

func TestRepos_SkipsDescentIntoFoundRepo(t *testing.T) {
	// root/repo/.git exists; root/repo/nested/.git should not be found
	root := buildTree(t, []string{
		"repo/.git",
		"repo/nested/.git",
	})

	got, err := Repos(root, 5)
	require.NoError(t, err)

	assert.Equal(t, []string{filepath.Join(root, "repo")}, got)
}

func TestRepos_NoRepos_ReturnsEmpty(t *testing.T) {
	root := buildTree(t, []string{"a/b/c"})

	got, err := Repos(root, 3)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRepos_RootDoesNotExist_ReturnsError(t *testing.T) {
	_, err := Repos("/nonexistent/path/xyz", 2)
	assert.Error(t, err)
}

func TestRepos_IgnoresRegularFiles(t *testing.T) {
	root := buildTree(t, []string{"repo/.git"})
	// Place a regular file alongside the .git dir to exercise the !d.IsDir() branch.
	require.NoError(t, os.WriteFile(filepath.Join(root, "repo", "README.md"), []byte("hi"), 0o644))

	got, err := Repos(root, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "repo")}, got)
}

func TestRepos_FindsGitFileWorktree(t *testing.T) {
	// A git worktree (and submodule) records its real git dir via a `.git`
	// FILE, not a directory. Such checkouts must still be discovered.
	root := buildTree(t, []string{"wt"})
	require.NoError(
		t,
		os.WriteFile(filepath.Join(root, "wt", ".git"), []byte("gitdir: /elsewhere\n"), 0o644),
	)

	got, err := Repos(root, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "wt")}, got)
}

func TestRepos_GitFileStopsDescent(t *testing.T) {
	// A repo marked by a `.git` file must not have its subdirectories scanned
	// for further repos.
	root := buildTree(t, []string{"wt/nested"})
	require.NoError(
		t,
		os.WriteFile(filepath.Join(root, "wt", ".git"), []byte("gitdir: /elsewhere\n"), 0o644),
	)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "wt", "nested", ".git"), 0o755))

	got, err := Repos(root, 5)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "wt")}, got)
}
