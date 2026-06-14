package worktreepath

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFor_HTTPSRemote(t *testing.T) {
	path, err := For("/crow", "https://github.com/acme/my-repo.git", "ws-abc")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo/workspaces/ws-abc", path)
}

func TestFor_HTTPSNoGitSuffix(t *testing.T) {
	path, err := For("/crow", "https://github.com/acme/my-repo", "ws-xyz")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo/workspaces/ws-xyz", path)
}

func TestFor_SSHRemote(t *testing.T) {
	path, err := For("/crow", "git@github.com:acme/my-repo.git", "ws-001")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo/workspaces/ws-001", path)
}

func TestFor_EmptyRemoteURLErrors(t *testing.T) {
	_, err := For("/crow", "", "ws-001")
	assert.Error(t, err)
}

func TestFor_UnrecognisedURLErrors(t *testing.T) {
	_, err := For("/crow", "not-a-url", "ws-001")
	assert.Error(t, err)
}

func TestFor_DeterministicSameInputs(t *testing.T) {
	a, err := For("/crow", "https://github.com/acme/repo.git", "ws-1")
	require.NoError(t, err)
	b, err := For("/crow", "https://github.com/acme/repo.git", "ws-1")
	require.NoError(t, err)
	assert.Equal(t, a, b)
}

func TestFor_DifferentWorkspacesDiverge(t *testing.T) {
	a, _ := For("/crow", "https://github.com/acme/repo.git", "ws-1")
	b, _ := For("/crow", "https://github.com/acme/repo.git", "ws-2")
	assert.NotEqual(t, a, b)
}

func TestRepoDir_HTTPS(t *testing.T) {
	dir, err := RepoDir("/crow", "https://github.com/acme/my-repo.git")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo", dir)
}

func TestRepoDir_SSH(t *testing.T) {
	dir, err := RepoDir("/crow", "git@github.com:acme/my-repo.git")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo", dir)
}

func TestRepoDir_EmptyErrors(t *testing.T) {
	_, err := RepoDir("/crow", "")
	assert.Error(t, err)
}

func TestRepoRelPath_StripsTrailingSlash(t *testing.T) {
	path, err := For("/crow", "https://github.com/acme/repo", "ws-1")
	require.NoError(t, err)
	assert.False(t, strings.Contains(path, "//"))
	assert.True(t, strings.HasPrefix(path, filepath.Join("/crow", "projects")))
}
