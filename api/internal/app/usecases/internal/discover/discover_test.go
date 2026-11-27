package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	require.NoError(t, os.WriteFile(filepath.Join(root, "repo", "README.md"), []byte("hi"), 0o644))

	got, err := Repos(root, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "repo")}, got)
}

func TestRepos_SkipsGitFileWorktree(t *testing.T) {
	root := buildTree(t, []string{"wt"})
	require.NoError(
		t,
		os.WriteFile(filepath.Join(root, "wt", ".git"), []byte("gitdir: /elsewhere\n"), 0o644),
	)

	got, err := Repos(root, 2)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRepos_GitFileStopsDescent(t *testing.T) {
	root := buildTree(t, []string{"wt/nested"})
	require.NoError(
		t,
		os.WriteFile(filepath.Join(root, "wt", ".git"), []byte("gitdir: /elsewhere\n"), 0o644),
	)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "wt", "nested", ".git"), 0o755))

	got, err := Repos(root, 5)
	require.NoError(t, err)
	assert.Empty(t, got)
}
