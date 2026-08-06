package layout

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recorder struct {
	moved map[string]string
	err   error
}

func newRecorder() *recorder { return &recorder{moved: map[string]string{}} }

func (r *recorder) Relocate(_ context.Context, id string, path string) error {
	if r.err != nil {
		return r.err
	}
	r.moved[id] = path
	return nil
}

type repairer struct{ repaired map[string]string }

func (g *repairer) WorktreeRepair(_ context.Context, repo string, wt string) error {
	if g.repaired == nil {
		g.repaired = map[string]string{}
	}
	g.repaired[repo] = wt
	return nil
}

// legacyWorkspace lays a workspace out the old way: a real directory named after
// its branch, holding worktree/ and chats/.
func legacyWorkspace(t *testing.T, home, project, slug, branch string) string {
	t.Helper()
	root := filepath.Join(home, "projects", project, slug, filepath.FromSlash(branch))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats", "c1"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "chats", "c1", "ledger"), []byte("history"), 0o600))
	return filepath.Join(root, "worktree")
}

func TestRun_MovesTheWorkspaceToItsIdentityAndLeavesAnAlias(t *testing.T) {
	home := t.TempDir()
	wt := legacyWorkspace(t, home, "p1", "github.com/acme/repo", "feature/x")
	rec, git := newRecorder(), &repairer{}

	res := Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: wt, RepoPath: "/repo"}}, git, rec)

	assert.Equal(t, 1, res.Migrated)
	newRoot := filepath.Join(home, "projects", "p1", "workspaces", "w1")
	assert.DirExists(t, filepath.Join(newRoot, "worktree"))

	// The chats travelled with the root — they are inside it, not beside it.
	body, err := os.ReadFile(filepath.Join(newRoot, "chats", "c1", "ledger"))
	require.NoError(t, err)
	assert.Equal(t, "history", string(body))

	// The old path is now a symlink, so every absolute path already recorded
	// against it keeps resolving without being rewritten.
	oldRoot := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "feature", "x")
	target, err := os.Readlink(oldRoot)
	require.NoError(t, err)
	assert.Equal(t, newRoot, target)
	assert.DirExists(t, filepath.Join(oldRoot, "worktree"), "the old path still resolves")

	assert.Equal(t, filepath.Join(newRoot, "worktree"), rec.moved["w1"], "the record follows")
	assert.Equal(t, filepath.Join(newRoot, "worktree"), git.repaired["/repo"])
}

// An adopted checkout is the user's OWN repository, outside crowbar home.
func TestRun_NeverTouchesAnAdoptedCheckout(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "my-repo")
	require.NoError(t, os.MkdirAll(outside, 0o755))
	rec := newRecorder()

	res := Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: outside}}, nil, rec)

	assert.Equal(t, 1, res.Skipped)
	assert.DirExists(t, outside, "the user's own repository must be left where it is")
	assert.Empty(t, rec.moved)
}

func TestRun_SkipsAnUnprovisionedPlaceholder(t *testing.T) {
	rec := newRecorder()
	res := Run(context.Background(), t.TempDir(),
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: ""}}, nil, rec)
	assert.Equal(t, 1, res.Skipped)
	assert.Empty(t, rec.moved)
}

func TestRun_IsIdempotent(t *testing.T) {
	home := t.TempDir()
	wt := legacyWorkspace(t, home, "p1", "slug", "main")
	rec := newRecorder()
	ws := []Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: wt}}

	first := Run(context.Background(), home, ws, nil, rec)
	require.Equal(t, 1, first.Migrated)

	// Re-run with the record now naming the NEW path, as it would on the next boot.
	second := Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: rec.moved["w1"]}}, nil, rec)

	assert.Equal(t, 1, second.Skipped)
	assert.Equal(t, 0, second.Migrated)
}

// An interruption between the move and the record write leaves the tree at the
// new root with the record still on the old one. The next boot must FINISH it,
// not skip it — otherwise the record names a path only the alias resolves,
// forever.
func TestRun_FinishesAMoveInterruptedBeforeTheRecordWrite(t *testing.T) {
	home := t.TempDir()
	wt := legacyWorkspace(t, home, "p1", "slug", "main")
	failing := newRecorder()
	failing.err = assert.AnError
	ws := []Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: wt}}

	crashed := Run(context.Background(), home, ws, nil, failing)
	require.Equal(t, 1, crashed.Failed, "the record write failed")

	// Same input on the next boot: the record still names the old path.
	rec := newRecorder()
	res := Run(context.Background(), home, ws, nil, rec)

	assert.Equal(t, 1, res.Migrated)
	assert.Equal(t,
		filepath.Join(home, "projects", "p1", "workspaces", "w1", "worktree"),
		rec.moved["w1"], "the record must be caught up to where the tree already is")
}

// Storages lived OUTSIDE the root, keyed by repo id. They have to be carried in,
// or the rm -rf of the root is not the whole footprint it is documented to be.
func TestRun_AdoptsTheLegacyStorageDirectory(t *testing.T) {
	home := t.TempDir()
	wt := legacyWorkspace(t, home, "p1", "slug", "main")
	legacy := filepath.Join(home, "projects", "p1", "r1", "workspaces", "w1", "storages")
	require.NoError(t, os.MkdirAll(legacy, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "blob"), []byte("x"), 0o600))
	rec := newRecorder()

	Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: wt}}, nil, rec)

	newRoot := filepath.Join(home, "projects", "p1", "workspaces", "w1")
	body, err := os.ReadFile(filepath.Join(newRoot, "storages", "blob"))
	require.NoError(t, err)
	assert.Equal(t, "x", string(body))
	assert.NoDirExists(t, filepath.Join(home, "projects", "p1", "r1", "workspaces", "w1"),
		"the legacy location must not survive")
}

// One wedged workspace must not stop the others, and must never fail boot.
func TestRun_KeepsGoingAfterOneFailure(t *testing.T) {
	home := t.TempDir()
	good := legacyWorkspace(t, home, "p1", "slug", "good")
	// A path under home that does not exist: the move fails.
	bad := filepath.Join(home, "projects", "p1", "slug", "missing", "worktree")
	rec := newRecorder()

	res := Run(context.Background(), home, []Workspace{
		{ID: "w-bad", ProjectID: "p1", WorktreePath: bad},
		{ID: "w-good", ProjectID: "p1", WorktreePath: good},
	}, nil, rec)

	assert.Equal(t, 1, res.Migrated)
	assert.Contains(t, rec.moved, "w-good", "a later workspace still gets converted")
}

// Regression: the guard was "is the worktree under crowbar home", which looks
// equivalent to "is it ours" and is not. An adopted checkout can live INSIDE the
// home — the dev seed keeps them at <home>/seed/<project> — and that guard
// migrated all three, replacing the user's repositories with symlinks. Only
// <home>/projects/<projectID>/... belongs to Crowbar.
func TestRegression_Run_SkipsAnAdoptedCheckoutThatLivesInsideCrowbarHome(t *testing.T) {
	home := t.TempDir()
	adopted := filepath.Join(home, "seed", "harbour", "infra")
	require.NoError(t, os.MkdirAll(filepath.Join(adopted, ".git"), 0o755))
	rec := newRecorder()

	res := Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: adopted}}, nil, rec)

	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 0, res.Migrated)
	info, err := os.Lstat(adopted)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink,
		"an adopted checkout must still be a real directory, not a link")
	assert.DirExists(t, filepath.Join(adopted, ".git"))
	assert.Empty(t, rec.moved)
}

// An adopted checkout is identified by a FACT from the repo/project record, not
// by where its directory happens to sit. This is the guard that matters: the
// path-shape version let three real repositories be moved.
func TestRun_NeverMovesAnAdoptedCheckoutEvenInsideTheProjectDir(t *testing.T) {
	home := t.TempDir()
	// Deliberately the worst case: an adopted checkout sitting exactly where a
	// managed worktree would live.
	adopted := filepath.Join(home, "projects", "p1", "slug", "main")
	require.NoError(t, os.MkdirAll(filepath.Join(adopted, ".git"), 0o755))
	rec := newRecorder()

	res := Run(context.Background(), home, []Workspace{
		{ID: "w1", ProjectID: "p1", WorktreePath: adopted, AdoptedPath: adopted},
	}, nil, rec)

	assert.Equal(t, 1, res.Skipped)
	info, err := os.Lstat(adopted)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "it must still be a real directory")
	assert.DirExists(t, filepath.Join(adopted, ".git"))
}

// A record that has drifted off an adopted checkout cannot be repaired by
// anything else: the path it names does not exist, so every later resolve and
// remove fails while the real directory sits there untouched. The repo/project
// record knows where it is, so the pass heals it rather than leaving a home
// needing hand surgery.
func TestRun_HealsAnAdoptedCheckoutWhoseRecordDrifted(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "seed", "harbour", "infra")
	require.NoError(t, os.MkdirAll(real, 0o755))
	stale := filepath.Join(home, "projects", "p1", "workspaces", "w1", "worktree")
	rec := newRecorder()

	res := Run(context.Background(), home, []Workspace{
		{ID: "w1", ProjectID: "p1", WorktreePath: stale, AdoptedPath: real},
	}, nil, rec)

	assert.Equal(t, 1, res.Migrated)
	assert.Equal(t, real, rec.moved["w1"], "the record must be put back on the real checkout")
}

// A managed worktree always ends in the "worktree" leaf. A record holding a bare
// directory would otherwise have filepath.Dir take its PARENT as the root — the
// arithmetic that turned one workspace move into a whole tree move.
func TestRegression_Run_RefusesAPathWithoutTheWorktreeLeaf(t *testing.T) {
	home := t.TempDir()
	bare := filepath.Join(home, "projects", "p1", "slug", "main")
	require.NoError(t, os.MkdirAll(bare, 0o755))
	rec := newRecorder()

	res := Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: bare}}, nil, rec)

	assert.Equal(t, 1, res.Skipped)
	assert.DirExists(t, bare)
	assert.NoDirExists(t, filepath.Join(home, "projects", "p1", "workspaces", "w1"))
	assert.Empty(t, rec.moved)
}

// Only a SYMLINK may be replaced when publishing an alias. A real directory at
// that path is a workspace nobody has migrated yet, and overwriting it would
// destroy a worktree.
func TestRun_RefusesToPublishOverARealDirectory(t *testing.T) {
	home := t.TempDir()
	wt := legacyWorkspace(t, home, "p1", "slug", "main")
	// Something already occupies the identity root, so the move fails.
	require.NoError(t, os.MkdirAll(
		filepath.Join(home, "projects", "p1", "workspaces", "w1"), 0o755))
	rec := newRecorder()

	res := Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: wt}}, nil, rec)

	assert.Equal(t, 1, res.Failed)
	assert.DirExists(t, filepath.Dir(wt), "the workspace must be left where it was")
	assert.Empty(t, rec.moved)
}

// An adopted checkout whose recorded path is already right needs nothing.
func TestRun_LeavesACorrectAdoptedRecordAlone(t *testing.T) {
	home := t.TempDir()
	adopted := filepath.Join(home, "seed", "proj")
	require.NoError(t, os.MkdirAll(adopted, 0o755))
	rec := newRecorder()

	res := Run(context.Background(), home, []Workspace{
		{ID: "w1", ProjectID: "p1", WorktreePath: adopted, AdoptedPath: adopted},
	}, nil, rec)

	assert.Equal(t, 1, res.Skipped)
	assert.Empty(t, rec.moved)
}

// The heal only fires when the real directory is actually there — otherwise it
// would point the record at a second path that does not exist either.
func TestRun_DoesNotHealOntoAMissingAdoptedPath(t *testing.T) {
	home := t.TempDir()
	rec := newRecorder()

	res := Run(context.Background(), home, []Workspace{{
		ID: "w1", ProjectID: "p1",
		WorktreePath: filepath.Join(home, "projects", "p1", "workspaces", "w1", "worktree"),
		AdoptedPath:  filepath.Join(home, "gone"),
	}}, nil, rec)

	assert.Equal(t, 1, res.Skipped)
	assert.Empty(t, rec.moved)
}

func TestRun_SkipsAWorkspaceWithNoProjectOrID(t *testing.T) {
	rec := newRecorder()
	res := Run(context.Background(), t.TempDir(), []Workspace{
		{ID: "", ProjectID: "p1", WorktreePath: "/x/worktree"},
		{ID: "w1", ProjectID: "", WorktreePath: "/x/worktree"},
	}, nil, rec)
	assert.Equal(t, 2, res.Skipped)
}

// The new root wins: a legacy storage entry whose name is already taken inside
// the root is left alone rather than clobbering what the workspace already has.
func TestRun_LegacyStorageNeverOverwritesWhatTheRootAlreadyHas(t *testing.T) {
	home := t.TempDir()
	wt := legacyWorkspace(t, home, "p1", "slug", "main")
	root := filepath.Dir(wt)
	require.NoError(t, os.WriteFile(filepath.Join(root, "keep.txt"), []byte("mine"), 0o600))
	legacy := filepath.Join(home, "projects", "p1", "r1", "workspaces", "w1")
	require.NoError(t, os.MkdirAll(legacy, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "keep.txt"), []byte("theirs"), 0o600))
	rec := newRecorder()

	Run(context.Background(), home,
		[]Workspace{{ID: "w1", ProjectID: "p1", WorktreePath: wt}}, nil, rec)

	newRoot := filepath.Join(home, "projects", "p1", "workspaces", "w1")
	body, err := os.ReadFile(filepath.Join(newRoot, "keep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "mine", string(body), "the root's own file must survive the adoption")
}

// A run with nothing to do touches nothing and says so.
func TestRun_EmptyInputIsANoOp(t *testing.T) {
	res := Run(context.Background(), t.TempDir(), nil, nil, newRecorder())
	assert.Equal(t, Result{}, res)
}

// underRoot is the boundary every guard leans on: a prefix that merely shares
// characters is not containment.
func TestUnderRoot_RequiresADirectoryBoundary(t *testing.T) {
	assert.True(t, underRoot("/home/projects/p1/x", "/home/projects"))
	assert.False(t, underRoot("/home/projects-other/x", "/home/projects"),
		"a shared prefix is not a parent directory")
	assert.False(t, underRoot("/home/projects", "/home/projects"), "a root is not under itself")
	assert.False(t, underRoot("", "/home"))
	assert.False(t, underRoot("/home/x", ""))
}
