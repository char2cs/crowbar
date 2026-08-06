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
	dir := StorageDir("/crow", "proj-1", "ws-abc")
	assert.Equal(
		t,
		"/crow/projects/proj-1/workspaces/ws-abc/storages",
		dir,
	)
}

func TestThreadsStorageDir(t *testing.T) {
	dir := ThreadsStorageDir("/crow", "proj-1", "ws-abc")
	assert.Equal(
		t,
		"/crow/projects/proj-1/workspaces/ws-abc/threads/storages",
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
			"/h/projects/proj/github.com/char2cs/crowbar/main/worktree",
		},
		{
			"proj",
			"github.com/char2cs/crowbar",
			"feature/login",
			"/h/projects/proj/github.com/char2cs/crowbar/feature/login/worktree",
		},
		// no-remote single-leaf name occupies the whole slug position.
		{
			"proj",
			"localrepo",
			"main",
			"/h/projects/proj/localrepo/main/worktree",
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

// Derive splices caller-supplied slug and branch straight into filepath.Join,
// which CLEANS the result — so a traversal component in either escapes the
// crowbar home entirely. A workspace out there can never be reclaimed: every
// removal guard refuses to touch anything that is not strictly under home.
func TestDerive_RejectsPathsThatEscapeHome(t *testing.T) {
	cases := []struct{ name, project, slug, branch string }{
		{"traversal in slug", "proj", "../../../../tmp/pwned", "main"},
		{"traversal in project", "../..", "slug", "main"},
		{"traversal in branch", "proj", "slug", "../../../../../../tmp/pwned"},
		{"slug climbs exactly to home", "proj", "../..", "main"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Derive("/h/.crowbar", c.project, c.slug, c.branch)
			require.Error(t, err, "derived %q", got)
			require.Empty(t, got)
		})
	}
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

// --- Workspace-root split: sibling worktree/ + chats/ (Task 1, spec §3.5) ---

func TestDerive_AppendsWorktreeLeaf(t *testing.T) {
	got, err := Derive("/home/.crowbar", "proj1", "github.com/acme/repo", "feat-x")
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	want := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x/worktree"
	if got != want {
		t.Fatalf("Derive = %q, want %q", got, want)
	}
}

func TestChatsAndLedgerAndRunnerDirs_SiblingOfWorktree(t *testing.T) {
	wt := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x/worktree"
	root := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x"

	if got := WorkspaceRoot(wt); got != root {
		t.Fatalf("WorkspaceRoot = %q, want %q", got, root)
	}
	chats := ChatsDir(wt)
	if want := root + "/chats"; chats != want {
		t.Fatalf("ChatsDir = %q, want %q", chats, want)
	}
	// AgentLedgerDir/RunnerDir take the resolved chats dir (Task 7): the caller
	// decides where chats live (sibling of a managed worktree, or rerooted under home
	// for an adopted checkout), and these helpers only hang the leaves off it.
	if got, want := AgentLedgerDir(chats, "chatA"), root+"/chats/chatA/ledger"; got != want {
		t.Fatalf("AgentLedgerDir = %q, want %q", got, want)
	}
	// The runner's tmp dir hangs off the CHATS dir, not off a chat: it is keyed by the
	// runner id + provider, the two things a Displace cannot erase, so boot
	// reconciliation can still find it for a runner that is placed nowhere.
	if got, want := RunnerDir(chats, "run1", "claude"), root+"/chats/runners/run1-claude"; got != want {
		t.Fatalf("RunnerDir = %q, want %q", got, want)
	}
}

// TestRunnerDir_DoesNotDependOnTheChat is the whole point of the layout, pinned: two
// runners of the same chat get distinct dirs, and one runner's dir is the SAME string
// however its chat pointer changes (or is erased). A path that varied with the chat could
// not be derived from a displaced runner's row at all — and a displaced runner whose kill
// failed is exactly the crash orphan boot reconciliation has to reap.
func TestRunnerDir_DoesNotDependOnTheChat(t *testing.T) {
	chats := "/home/.crowbar/projects/p/slug/branch/chats"

	if a, b := RunnerDir(chats, "run1", "claude"), RunnerDir(chats, "run2", "claude"); a == b {
		t.Fatalf("two runners must not share a tmp dir: both = %q", a)
	}
	if a, b := RunnerDir(chats, "run1", "claude"), RunnerDir(chats, "run1", "codex"); a == b {
		t.Fatalf("a re-spawn under another provider must not share a tmp dir: both = %q", a)
	}
	if got, want := RunnerDir(chats, "run1", "claude"), chats+"/runners/run1-claude"; got != want {
		t.Fatalf("RunnerDir = %q, want %q — derivable from the runner alone", got, want)
	}
}

// TestHomeDefaultChatsDir pins the rerooted-under-home layout for an adopted
// checkout (repo-home / project-home) whose worktree is the user's REAL dir
// OUTSIDE crowbar home: its agent chats go under a `default/` folder at the
// human-readable repo directory, so no plaintext ledger ever lands on the user's
// filesystem (Task 7).
func TestHomeDefaultChatsDir(t *testing.T) {
	got := HomeDefaultChatsDir("/home/.crowbar", "proj1", "github.com/acme/repo")
	want := "/home/.crowbar/projects/proj1/github.com/acme/repo/default/chats"
	if got != want {
		t.Fatalf("HomeDefaultChatsDir = %q, want %q", got, want)
	}
}

// TestHomeDefaultChatsDir_NoSlug covers the project-home (no repo, so no slug):
// its chats still root strictly under crowbar home at the project directory, never
// beside the user's real project folder.
func TestHomeDefaultChatsDir_NoSlug(t *testing.T) {
	got := HomeDefaultChatsDir("/home/.crowbar", "proj1", "")
	want := "/home/.crowbar/projects/proj1/default/chats"
	if got != want {
		t.Fatalf("HomeDefaultChatsDir(no slug) = %q, want %q", got, want)
	}
}

// TestUnderHome is the shared predicate the chats-dir seam uses to tell a
// Crowbar-managed worktree (chats stay beside it) from an adopted checkout at the
// user's real dir (chats reroot under home). Only a path STRICTLY nested under
// home counts; home itself, a blank path, or a sibling that merely shares a
// prefix string does not.
func TestUnderHome(t *testing.T) {
	home := "/home/.crowbar"
	cases := []struct {
		path string
		want bool
	}{
		{"/home/.crowbar/projects/p/slug/branch/worktree", true},
		{"/home/.crowbar", false},           // home itself is not strictly under home
		{"/home/.crowbar-sibling/x", false}, // prefix-string match but not nested
		{"/Users/me/real-repo", false},      // an adopted checkout outside home
		{"", false},
	}
	for _, c := range cases {
		if got := UnderHome(c.path, home); got != c.want {
			t.Fatalf("UnderHome(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
