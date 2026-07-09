package worktreepath

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

func TestStorageDir(t *testing.T) {
	dir := StorageDir("/crow", "proj-1", "repo-1", "ws-abc")
	assert.Equal(
		t,
		"/crow/projects/proj-1/repo-1/workspaces/ws-abc/storages",
		dir,
	)
}

func TestAgentLedgerDir(t *testing.T) {
	dir := AgentLedgerDir("/crow", "proj-1", "repo-1", "ws-abc", "chat-1")
	assert.Equal(
		t,
		"/crow/projects/proj-1/repo-1/workspaces/ws-abc/agent-ledger/chat-1",
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

func TestDefaultCrowbarHome(t *testing.T) {
	t.Setenv(metadata.HomeEnvVar, "")
	t.Setenv("HOME", "/home/tester")
	home, err := DefaultCrowbarHome()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/tester", ".crowbar"), home)
}

func TestDefaultCrowbarHome_EnvOverride(t *testing.T) {
	t.Setenv(metadata.HomeEnvVar, "/tmp/dev-crowbar-home")
	home, err := DefaultCrowbarHome()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/dev-crowbar-home", home)
}

// --- Human-readable worktree path derivation (Task 3, spec §3.9) ---

func TestDerive(t *testing.T) {
	home := "/h"
	cases := []struct{ project, slug, branch, want string }{
		{
			"proj",
			"github.com/char2cs/crowbar",
			"main",
			"/h/projects/proj/github.com/char2cs/crowbar/main",
		},
		{
			"proj",
			"github.com/char2cs/crowbar",
			"feature/login",
			"/h/projects/proj/github.com/char2cs/crowbar/feature/login",
		},
		// no-remote single-leaf name occupies the whole slug position.
		{
			"proj",
			"localrepo",
			"main",
			"/h/projects/proj/localrepo/main",
		},
	}
	for _, c := range cases {
		got, err := Derive(home, c.project, c.slug, c.branch)
		require.NoError(t, err)
		assert.Equal(t, c.want, got)
	}
}

func TestDerive_RejectsEmptyComponent(t *testing.T) {
	cases := []struct{ home, project, slug, branch string }{
		{"", "proj", "slug", "main"},
		{"/h", "", "slug", "main"},
		{"/h", "proj", "", "main"},
		{"/h", "proj", "slug", ""},
	}
	for _, c := range cases {
		_, err := Derive(c.home, c.project, c.slug, c.branch)
		require.Error(t, err)
	}
}

func TestHomeLeafIsDotHome(t *testing.T) {
	got := HomeLeaf("/h", "proj", "github.com/o/r")
	assert.Equal(t, "/h/projects/proj/github.com/o/r/.home", got)
}

func TestDetectClash_CaseInsensitive(t *testing.T) {
	existing := []string{"/h/projects/p/github.com/o/Repo/main"}
	err := DetectClash(existing, "/h/projects/p/github.com/o/repo/main")
	require.Error(t, err) // rejected on a case-insensitive FS
	require.ErrorIs(t, err, ErrPathClash)
}

func TestDetectClash_NoClash(t *testing.T) {
	existing := []string{"/h/projects/p/github.com/o/repo/main"}
	err := DetectClash(existing, "/h/projects/p/github.com/o/repo/feature")
	require.NoError(t, err)
}

func TestMove_Success(t *testing.T) {
	var gitCalls, mapCalls int
	err := Move(
		"/old",
		"/new",
		func(from, to string) error {
			gitCalls++
			assert.Equal(t, "/old", from)
			assert.Equal(t, "/new", to)
			return nil
		},
		func() error { mapCalls++; return nil },
	)
	require.NoError(t, err)
	assert.Equal(t, 1, gitCalls)
	assert.Equal(t, 1, mapCalls)
}

func TestMove_GitFailure_KeepsOldMapEntry(t *testing.T) {
	sentinel := errors.New("git move failed")
	mapCalled := false
	err := Move(
		"/old",
		"/new",
		func(from, to string) error { return sentinel },
		func() error { mapCalled = true; return nil },
	)
	require.ErrorIs(t, err, sentinel)
	assert.False(t, mapCalled, "map must not update when git move fails")
}

func TestMove_MapUpdateFailure(t *testing.T) {
	sentinel := errors.New("map update failed")
	err := Move(
		"/old",
		"/new",
		func(from, to string) error { return nil },
		func() error { return sentinel },
	)
	require.ErrorIs(t, err, sentinel)
}
