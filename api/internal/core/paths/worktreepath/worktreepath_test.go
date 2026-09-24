package worktreepath

import (
	"os"
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

func TestChatsAndRunnerDirs_SiblingOfWorktree(t *testing.T) {
	wt := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x/worktree"
	root := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x"

	if got := WorkspaceRoot(wt); got != root {
		t.Fatalf("WorkspaceRoot = %q, want %q", got, root)
	}
	chats := ChatsDir(wt)
	if want := root + "/chats"; chats != want {
		t.Fatalf("ChatsDir = %q, want %q", chats, want)
	}
	// RunnerDir takes the resolved chats dir (Task 7): the caller decides where
	// chats live (sibling of a managed worktree, or rerooted under home for an
	// adopted checkout), and this helper only hangs the leaf off it.
	//
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

// TestLedgerChatsDir_IgnoresWorkspaceEntirely pins spec §1.5: a chat's own
// ledger root takes crowbar home and NOTHING else — no worktree path, no
// workspace id — because WorkspaceID is optional and mutable and must never
// move it.
func TestLedgerChatsDir_IgnoresWorkspaceEntirely(t *testing.T) {
	got := LedgerChatsDir("/home/.crowbar")
	want := "/home/.crowbar/chats"
	if got != want {
		t.Fatalf("LedgerChatsDir = %q, want %q", got, want)
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

func TestFreePathBranch_UsesTheBranchWhenTheNameIsFree(t *testing.T) {
	home := t.TempDir()

	got, err := FreePathBranch(home, "p1", "github.com/o/r", "main", nil)

	require.NoError(t, err)
	assert.Equal(t, "main", got)
}

// A name frozen by a workspace created under it — which never follows that
// workspace's later branch renames — must not block a new workspace.
func TestFreePathBranch_SuffixesAFrozenName(t *testing.T) {
	home := t.TempDir()
	slug := "github.com/o/r"
	root := filepath.Join(home, "projects", "p1", filepath.FromSlash(slug), "main")
	require.NoError(t, os.MkdirAll(root, 0o755))

	got, err := FreePathBranch(home, "p1", slug, "main", []string{root})

	require.NoError(t, err)
	assert.Equal(t, "main-2", got)
}

func TestFreePathBranch_KeepsCountingPastAnOccupiedSuffix(t *testing.T) {
	home := t.TempDir()
	slug := "github.com/o/r"
	base := filepath.Join(home, "projects", "p1", filepath.FromSlash(slug))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "main"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "main-2"), 0o755))

	got, err := FreePathBranch(home, "p1", slug, "main", nil)

	require.NoError(t, err)
	assert.Equal(t, "main-3", got)
}

// TestRegression_FreePathBranch_SeesANestedBranch pins the gap the sibling scan
// alone leaves: siblingWorktreePaths lists only the TOP-LEVEL entries under the
// slug, so a nested root like feature/x is never in it and DetectClash can never
// match. The directory test is what covers that depth.
func TestRegression_FreePathBranch_SeesANestedBranch(t *testing.T) {
	home := t.TempDir()
	slug := "github.com/o/r"
	nested := filepath.Join(home, "projects", "p1", filepath.FromSlash(slug), "feature", "x")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	// The sibling list is what the real scanner would produce: top level only.
	siblings := []string{filepath.Join(home, "projects", "p1", filepath.FromSlash(slug), "feature")}

	got, err := FreePathBranch(home, "p1", slug, "feature/x", siblings)

	require.NoError(t, err)
	assert.Equal(t, "feature/x-2", got)
}

// --- Attachment durable store (Task 1) ---

func TestAttachmentsDir(t *testing.T) {
	dir := AttachmentsDir("/crow/projects/p1/slug/branch/chats", "chat-1")
	assert.Equal(t, "/crow/projects/p1/slug/branch/chats/chat-1/attachments", dir)
}

func TestRestoreDurableAttachmentRefs_RewritesAnAbsoluteReferenceBack(t *testing.T) {
	text := "look ![x](/crow/projects/p1/slug/branch/chats/chat-1/attachments/photo.png) done"
	out := RestoreDurableAttachmentRefs(text, "/crow/projects/p1/slug/branch/chats", "chat-1")
	assert.Equal(t, "look ![x](chats/chat-1/attachments/photo.png) done", out)
}

func TestRestoreDurableAttachmentRefs_LeavesUnrelatedTextUntouched(t *testing.T) {
	text := "nothing to restore here, and no /crow/projects/p1/slug/branch/chats/OTHER-CHAT/attachments/x.png reference either"
	out := RestoreDurableAttachmentRefs(text, "/crow/projects/p1/slug/branch/chats", "chat-1")
	assert.Equal(t, text, out, "a different chat's durable dir is not this chat's to rewrite")
}

func TestRestoreDurableAttachmentRefs_MultipleReferences(t *testing.T) {
	text := "![a](/crow/projects/p1/slug/branch/chats/chat-1/attachments/one.png) and ![b](/crow/projects/p1/slug/branch/chats/chat-1/attachments/two.png)"
	out := RestoreDurableAttachmentRefs(text, "/crow/projects/p1/slug/branch/chats", "chat-1")
	assert.Equal(t, "![a](chats/chat-1/attachments/one.png) and ![b](chats/chat-1/attachments/two.png)", out)
}

// Only the leaf shape names a directory that is one workspace's alone.
func TestOwnRoot_AcceptsOnlyTheLeafShapeInsideAProject(t *testing.T) {
	home := "/crow"
	root, ok := OwnRoot("/crow/projects/p/github.com/acme/app/main/worktree", home)
	require.True(t, ok)
	assert.Equal(t, "/crow/projects/p/github.com/acme/app/main", root)

	for _, path := range []string{
		"/crow/projects/p/github.com/acme/app/develop", // pre-leaf
		"/crow/projects/p/worktree",                    // root is the project dir
		"/crow/projects/worktree",                      // root is projects/
		"/crow/worktree",                               // root is the home
		"/elsewhere/projects/p/x/worktree",             // outside the home
		"/crow/projects/p/a/develop/../main/worktree",  // unclean
		"crow/projects/p/a/main/worktree",              // relative
		"",
	} {
		_, ok := OwnRoot(path, home)
		assert.False(t, ok, "must refuse %q", path)
	}
}

// A pre-leaf row keeps the <slug>/chats tree its attachments already live in,
// unless that tree is a sibling's checkout; a leaf row keeps its own.
func TestManagedChatsDir_KeepsAPreLeafRowsTreeUnlessItIsACheckout(t *testing.T) {
	home := t.TempDir()
	slug := filepath.Join(home, "projects", "p", "app")

	assert.Equal(t, filepath.Join(slug, "main", "chats"),
		ManagedChatsDir(home, "p", "w1", filepath.Join(slug, "main", "worktree")))
	assert.Equal(t, filepath.Join(slug, "chats"),
		ManagedChatsDir(home, "p", "w2", filepath.Join(slug, "develop")))

	require.NoError(t, os.MkdirAll(filepath.Join(slug, "chats"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(slug, "chats", ".git"), []byte("gitdir: x"), 0o644))
	assert.Equal(t, filepath.Join(home, "projects", "p", ".workspace-chats", "w2"),
		ManagedChatsDir(home, "p", "w2", filepath.Join(slug, "develop")),
		"a branch named chats is a checkout, never another workspace's chats tree")
	assert.Equal(t, filepath.Join(home, "projects", "p", ".workspace-chats", "w3"),
		ManagedChatsDir(home, "p", "w3", filepath.Join(home, "stray")),
		"a path outside the project never resolves beside it")
}

// A checkout that is, or holds, another row's checkout or chats tree holds
// that row's files; a checkout beside them does not.
func TestHoldsAnother(t *testing.T) {
	others := []string{"/h/p/app/dev", "/h/p/app/main/worktree", ""}
	assert.True(t, HoldsAnother("/h/p/app/chats", others), "dev's chats tree is <slug>/chats")
	assert.True(t, HoldsAnother("/h/p/app/dev", others), "the same checkout")
	assert.True(t, HoldsAnother("/h/p/app/main", others), "a checkout holding main's root")
	assert.False(t, HoldsAnother("/h/p/app/threads", others))
	assert.False(t, HoldsAnother("/h/p/app/main/worktree/x", others))
}
