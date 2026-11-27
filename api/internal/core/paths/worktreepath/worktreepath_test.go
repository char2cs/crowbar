package worktreepath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestStorageDir(t *testing.T) {
	dir := StorageDir("/crow", "proj-1", "repo-1", "ws-abc")
	assert.Equal(
		t,
		"/crow/projects/proj-1/repo-1/workspaces/ws-abc/storages",
		dir,
	)
}

func TestRepoDir(t *testing.T) {
	dir := RepoDir("/crow", "proj-1", "repo-1")
	assert.Equal(t, "/crow/projects/proj-1/repo-1", dir)
}

func TestRepoIconPath(t *testing.T) {
	dir := RepoIconPath("/crow", "proj-1", "repo-1")
	assert.Equal(t, "/crow/projects/proj-1/repo-1/icon", dir)
}

func TestProjectDir(t *testing.T) {
	dir := ProjectDir("/crow", "proj-1")
	assert.Equal(t, "/crow/projects/proj-1", dir)
}

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
	require.Error(t, err)
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
	if got, want := RunnerDir(chats, "run1", "claude"), root+"/chats/runners/run1-claude"; got != want {
		t.Fatalf("RunnerDir = %q, want %q", got, want)
	}
}

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

func TestHomeDefaultChatsDir(t *testing.T) {
	got := HomeDefaultChatsDir("/home/.crowbar", "proj1", "github.com/acme/repo")
	want := "/home/.crowbar/projects/proj1/github.com/acme/repo/default/chats"
	if got != want {
		t.Fatalf("HomeDefaultChatsDir = %q, want %q", got, want)
	}
}

func TestHomeDefaultChatsDir_NoSlug(t *testing.T) {
	got := HomeDefaultChatsDir("/home/.crowbar", "proj1", "")
	want := "/home/.crowbar/projects/proj1/default/chats"
	if got != want {
		t.Fatalf("HomeDefaultChatsDir(no slug) = %q, want %q", got, want)
	}
}

func TestLedgerChatsDir_IgnoresWorkspaceEntirely(t *testing.T) {
	got := LedgerChatsDir("/home/.crowbar")
	want := "/home/.crowbar/chats"
	if got != want {
		t.Fatalf("LedgerChatsDir = %q, want %q", got, want)
	}
}

func TestRemoveUnderHome_SaysWhyATargetStays(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	require.Error(t, RemoveUnderHome(home, outside))
	assert.DirExists(t, outside)

	removed := filepath.Join(home, "removed")
	require.NoError(t, os.MkdirAll(filepath.Join(removed, "inner"), 0o700))
	require.NoError(t, RemoveUnderHome(home, removed))
	assert.NoDirExists(t, removed)
	require.NoError(t, RemoveUnderHome(home, removed), "an absent target is already removed")
}

func TestRemoveUnderHome_ReportsARemovalThatFailed(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	home := t.TempDir()
	parent := filepath.Join(home, "parent")
	stuck := filepath.Join(parent, "stuck")
	require.NoError(t, os.MkdirAll(stuck, 0o700))
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0o700)) })

	require.ErrorIs(t, RemoveUnderHome(home, stuck), os.ErrPermission)
	assert.DirExists(t, stuck)
}

func TestUnderHome(t *testing.T) {
	home := "/home/.crowbar"
	cases := []struct {
		path string
		want bool
	}{
		{"/home/.crowbar/projects/p/slug/branch/worktree", true},
		{"/home/.crowbar", false},
		{"/home/.crowbar-sibling/x", false},
		{"/Users/me/real-repo", false},
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

func TestRegression_FreePathBranch_SeesANestedBranch(t *testing.T) {
	home := t.TempDir()
	slug := "github.com/o/r"
	nested := filepath.Join(home, "projects", "p1", filepath.FromSlash(slug), "feature", "x")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	siblings := []string{filepath.Join(home, "projects", "p1", filepath.FromSlash(slug), "feature")}

	got, err := FreePathBranch(home, "p1", slug, "feature/x", siblings)

	require.NoError(t, err)
	assert.Equal(t, "feature/x-2", got)
}

func TestAttachmentsDir(t *testing.T) {
	dir := AttachmentsDir("/crow", "chat-1")
	assert.Equal(t, "/crow/chats/chat-1/attachments", dir)
}

func TestRelayDir_IsTheChatsOwnAndNeverItsRunnersOrWorkspaces(t *testing.T) {
	assert.Equal(t, "/crow/chats/chat-1/relay", RelayDir("/crow", "chat-1"))
}

func TestRestoreDurableAttachmentRefs_RewritesAnAbsoluteReferenceBack(t *testing.T) {
	text := "look ![x](/crow/chats/chat-1/attachments/photo.png) done"
	out := RestoreDurableAttachmentRefs(text, "/crow", "chat-1")
	assert.Equal(t, "look ![x](chats/chat-1/attachments/photo.png) done", out)
}

func TestRestoreDurableAttachmentRefs_LeavesUnrelatedTextUntouched(t *testing.T) {
	text := "nothing to restore here, and no /crow/chats/OTHER-CHAT/attachments/x.png reference either"
	out := RestoreDurableAttachmentRefs(text, "/crow", "chat-1")
	assert.Equal(t, text, out, "a different chat's durable dir is not this chat's to rewrite")
}

func TestRestoreDurableAttachmentRefs_MultipleReferences(t *testing.T) {
	text := "![a](/crow/chats/chat-1/attachments/one.png) and ![b](/crow/chats/chat-1/attachments/two.png)"
	out := RestoreDurableAttachmentRefs(text, "/crow", "chat-1")
	assert.Equal(t, "![a](chats/chat-1/attachments/one.png) and ![b](chats/chat-1/attachments/two.png)", out)
}

func TestOwnRoot_AcceptsOnlyTheLeafShapeInsideAProject(t *testing.T) {
	home := "/crow"
	root, ok := OwnRoot("/crow/projects/p/github.com/acme/app/main/worktree", home)
	require.True(t, ok)
	assert.Equal(t, "/crow/projects/p/github.com/acme/app/main", root)

	for _, path := range []string{
		"/crow/projects/p/github.com/acme/app/develop",
		"/crow/projects/p/worktree",
		"/crow/projects/worktree",
		"/crow/worktree",
		"/elsewhere/projects/p/x/worktree",
		"/crow/projects/p/a/develop/../main/worktree",
		"crow/projects/p/a/main/worktree",
		"",
	} {
		_, ok := OwnRoot(path, home)
		assert.False(t, ok, "must refuse %q", path)
	}
}

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

func TestHoldsAnother(t *testing.T) {
	others := []string{"/h/p/app/dev", "/h/p/app/main/worktree", ""}
	assert.True(t, HoldsAnother("/h/p/app/chats", others), "dev's chats tree is <slug>/chats")
	assert.True(t, HoldsAnother("/h/p/app/dev", others), "the same checkout")
	assert.True(t, HoldsAnother("/h/p/app/main", others), "a checkout holding main's root")
	assert.False(t, HoldsAnother("/h/p/app/threads", others))
	assert.False(t, HoldsAnother("/h/p/app/main/worktree/x", others))
}
