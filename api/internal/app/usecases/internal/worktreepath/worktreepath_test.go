package worktreepath

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFor_UUIDPath(t *testing.T) {
	path := For("/crow", "proj-1", "repo-1", "ws-abc")
	assert.Equal(
		t,
		"/crow/projects/proj-1/repo-1/workspaces/ws-abc/worktree",
		path,
	)
}

func TestStorageDir(t *testing.T) {
	dir := StorageDir("/crow", "proj-1", "repo-1", "ws-abc")
	assert.Equal(
		t,
		"/crow/projects/proj-1/repo-1/workspaces/ws-abc/storages",
		dir,
	)
}

func TestThreadsStorageDir(t *testing.T) {
	dir := ThreadsStorageDir("/crow", "proj-1", "repo-1", "ws-abc")
	assert.Equal(
		t,
		"/crow/projects/proj-1/repo-1/workspaces/ws-abc/threads/storages",
		dir,
	)
}

func TestRepoDir(t *testing.T) {
	dir := RepoDir("/crow", "proj-1", "repo-1")
	assert.Equal(t, "/crow/projects/proj-1/repo-1", dir)
}

func TestRepoStorageDir(t *testing.T) {
	dir := RepoStorageDir("/crow", "proj-1", "repo-1")
	assert.Equal(t, "/crow/projects/proj-1/repo-1/storages", dir)
}

func TestRepoIconPath(t *testing.T) {
	dir := RepoIconPath("/crow", "proj-1", "repo-1")
	assert.Equal(t, "/crow/projects/proj-1/repo-1/icon", dir)
}

func TestProjectDir(t *testing.T) {
	dir := ProjectDir("/crow", "proj-1")
	assert.Equal(t, "/crow/projects/proj-1", dir)
}

func TestProjectStorageDir(t *testing.T) {
	dir := ProjectStorageDir("/crow", "proj-1")
	assert.Equal(t, "/crow/projects/proj-1/storages", dir)
}

func TestGlobalStateDir(t *testing.T) {
	dir := GlobalStateDir("/crow")
	assert.Equal(t, "/crow/state", dir)
}

func TestFor_Deterministic(t *testing.T) {
	a := For("/crow", "proj-1", "repo-1", "ws-1")
	b := For("/crow", "proj-1", "repo-1", "ws-1")
	assert.Equal(t, a, b)
}

func TestFor_DivergesByWorkspace(t *testing.T) {
	a := For("/crow", "proj-1", "repo-1", "ws-1")
	b := For("/crow", "proj-1", "repo-1", "ws-2")
	assert.NotEqual(t, a, b)
}

func TestDefaultCrowbarHome(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	home, err := DefaultCrowbarHome()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/tester", ".crowbar"), home)
}
