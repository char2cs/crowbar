package worktree_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Both git seams the rename drives honour their context, because the real ones
// do: every git invocation runs through exec.CommandContext, so a cancelled
// context kills the command. Modelling that is what makes a client disconnect
// visible to these tests at all.
func (f *fakeGit) RenameBranch(
	ctx context.Context,
	repoPath string,
	oldName string,
	newName string,
) error {
	f.record("RenameBranch", repoPath, oldName, newName)
	if err := ctx.Err(); err != nil {
		return err
	}
	f.branchNow = newName
	if f.onBranchRenamed != nil {
		hook := f.onBranchRenamed
		f.onBranchRenamed = nil
		hook()
	}
	return nil
}

func (f *fakeGit) WorktreeRepair(
	ctx context.Context,
	repoPath string,
	worktreePath string,
) error {
	f.record("WorktreeRepair", repoPath, worktreePath)
	return ctx.Err()
}

// lastRenameTo returns the target name of the most recent RenameBranch call.
func (f *fakeGit) lastRenameTo() string {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].op == "RenameBranch" {
			return f.calls[i].args[2]
		}
	}
	return ""
}

// renameFixture builds a usecase around one managed workspace on `branch`,
// with its workspace root materialised under a temp crowbar home.
type renameFixture struct {
	uc      worktree.Usecase
	git     *fakeGit
	home    string
	oldRoot string
	// recordErr makes the aggregate's record write fail, the one step that runs
	// after git AND the disk have both already moved.
	recordErr error
	renamed   struct {
		id, branch, worktreePath string
		called                   bool
	}
}

func newRenameFixture(t *testing.T, branch string, all []domain.Workspace) *renameFixture {
	t.Helper()
	home := t.TempDir()
	slugDir := filepath.Join(home, "projects", "p1", "github.com", "test", "repo")
	oldRoot := filepath.Join(slugDir, filepath.FromSlash(branch))
	require.NoError(t, os.MkdirAll(filepath.Join(oldRoot, "worktree"), 0o755))

	f := &renameFixture{git: &fakeGit{branchNow: branch}, home: home, oldRoot: oldRoot}

	self := domain.Workspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: branch,
		WorktreePath: filepath.Join(oldRoot, "worktree"),
		Status:       domain.WorkspaceStatusNew,
	}
	if len(all) == 0 {
		all = []domain.Workspace{self}
	}

	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			for _, w := range all {
				if w.ID == id {
					return w, nil
				}
			}
			return domain.Workspace{}, apperr.ErrNotFound
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		RenameBranchFn: func(id, b, p string) (domain.Workspace, error) {
			f.renamed.called = true
			f.renamed.id, f.renamed.branch, f.renamed.worktreePath = id, b, p
			if f.recordErr != nil {
				return domain.Workspace{}, f.recordErr
			}
			return domain.Workspace{ID: id, Branch: b, WorktreePath: p}, nil
		},
	}
	f.uc = worktree.New(ws, f.git, &fakeProvider{},
		&fakeRepoStore{path: "/repo", remoteURL: "https://github.com/test/repo.git"},
		newNow(), func() (string, error) { return home, nil })
	return f
}

func TestRenameBranch_RenamesBranchMovesRootAndRecordsBoth(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)

	got, err := f.uc.RenameBranch(context.Background(), "w1", "feature/x")
	require.NoError(t, err)

	newRoot := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo", "feature", "x")
	newWorktree := filepath.Join(newRoot, "worktree")

	// git was told to rename the branch and then to repair the relocated tree.
	assert.Contains(t, f.git.ops(), "RenameBranch")
	assert.Contains(t, f.git.ops(), "WorktreeRepair")

	// The whole workspace root moved, not just the worktree leaf.
	assert.NoDirExists(t, f.oldRoot, "old workspace root must not be left behind")
	assert.DirExists(t, newWorktree)

	// The record follows git, carrying both halves.
	assert.True(t, f.renamed.called, "aggregate must record the rename")
	assert.Equal(t, "feature/x", f.renamed.branch)
	assert.Equal(t, newWorktree, f.renamed.worktreePath)
	assert.Equal(t, "feature/x", got.Branch)
	assert.Equal(t, newWorktree, got.WorktreePath)
}

// The agent chats tree is a SIBLING of the worktree inside the workspace root.
// Moving only the worktree leaf would orphan a workspace's whole chat history
// at the old branch's path.
func TestRenameBranch_CarriesAgentChatsWithTheWorkspace(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	chats := filepath.Join(f.oldRoot, "chats")
	require.NoError(t, os.MkdirAll(chats, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(chats, "c1.json"), []byte("history"), 0o600))

	_, err := f.uc.RenameBranch(context.Background(), "w1", "feature/x")
	require.NoError(t, err)

	moved := filepath.Join(
		f.home, "projects", "p1", "github.com", "test", "repo", "feature", "x", "chats", "c1.json")
	body, readErr := os.ReadFile(moved)
	require.NoError(t, readErr, "chat history must travel with the workspace root")
	assert.Equal(t, "history", string(body))
}

func TestRenameBranch_RejectsLockedWorkspaceBeforeTouchingGit(t *testing.T) {
	locked := domain.Workspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main",
		WorktreePath: "/anything/worktree", Status: domain.WorkspaceStatusLocked,
	}
	f := newRenameFixture(t, "main", []domain.Workspace{locked})

	_, err := f.uc.RenameBranch(context.Background(), "w1", "feature/x")

	require.ErrorIs(t, err, worktree.ErrWorkspaceLocked)
	assert.Empty(t, f.git.ops(), "a locked workspace must be refused before any git work")
	assert.False(t, f.renamed.called)
}

// The repo home is an adopted checkout at the user's own directory, outside
// crowbar home. Renaming its branch must never relocate that directory.
func TestRenameBranch_RejectsWorkspaceOutsideCrowbarHome(t *testing.T) {
	// An adopted workspace living OUTSIDE crowbar home, in a real directory this
	// test owns. It has to be real, and it has to be ours: the point of the
	// assertion below is that a refused rename touches nothing on disk, and the
	// only way to mean that is to check a directory that would actually have been
	// moved. It is a sibling of the fixture's own t.TempDir() home, so it is
	// genuinely unmanaged.
	//
	// This used to name "/Users/someone/Projects/athas" and assert "/Users" still
	// existed — which asserts nothing on a machine that has no /Users, and worse,
	// FAILED there. It passed on macOS and went red on Linux CI.
	outside := t.TempDir()
	tree := filepath.Join(outside, "Projects", "athas")
	require.NoError(t, os.MkdirAll(tree, 0o755))

	adopted := domain.Workspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "master",
		WorktreePath: tree, Status: domain.WorkspaceStatusNew,
		IsDefault: true,
	}
	f := newRenameFixture(t, "master", []domain.Workspace{adopted})

	_, err := f.uc.RenameBranch(context.Background(), "w1", "feature/x")

	require.ErrorIs(t, err, worktree.ErrRenameUnmanagedWorkspace)
	assert.Empty(t, f.git.ops())
	assert.DirExists(t, tree, "an unmanaged workspace's tree must be left untouched")
}

func TestRenameBranch_RejectsBranchAnotherWorkspaceHolds(t *testing.T) {
	home := t.TempDir()
	slugDir := filepath.Join(home, "projects", "p1", "github.com", "test", "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(slugDir, "testing", "worktree"), 0o755))
	self := domain.Workspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "testing",
		WorktreePath: filepath.Join(slugDir, "testing", "worktree"),
		Status:       domain.WorkspaceStatusNew,
	}
	sibling := domain.Workspace{
		ID: "w2", RepoID: "r1", ProjectID: "p1", Branch: "taken",
		WorktreePath: filepath.Join(slugDir, "taken", "worktree"),
		Status:       domain.WorkspaceStatusNew,
	}
	f := newRenameFixture(t, "testing", []domain.Workspace{self, sibling})

	_, err := f.uc.RenameBranch(context.Background(), "w1", "taken")

	require.ErrorIs(t, err, worktree.ErrBranchWorkspaceExists)
	assert.Empty(t, f.git.ops())
}

func TestRenameBranch_RejectsOccupiedDestinationDirectory(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	// A leftover directory sits exactly where the rename wants to land.
	occupied := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo", "occupied")
	require.NoError(t, os.MkdirAll(occupied, 0o755))

	_, err := f.uc.RenameBranch(context.Background(), "w1", "occupied")

	require.ErrorIs(t, err, worktree.ErrRenameTargetExists)
	assert.Empty(t, f.git.ops(), "the clash must be caught before any git work")
	assert.DirExists(t, filepath.Join(f.oldRoot, "worktree"), "the workspace must stay put")
}

// A case-only difference is a distinct branch to git but the same directory on
// APFS, so it must be refused rather than silently merged into the old tree.
func TestRenameBranch_RejectsCaseOnlyClashWithSibling(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	slugDir := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(slugDir, "Feature"), 0o755))

	_, err := f.uc.RenameBranch(context.Background(), "w1", "feature")

	require.Error(t, err)
	assert.Empty(t, f.git.ops())
}

func TestRenameBranch_SameNameIsANoOp(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)

	got, err := f.uc.RenameBranch(context.Background(), "w1", "testing")

	require.NoError(t, err)
	assert.Equal(t, "testing", got.Branch)
	assert.Empty(t, f.git.ops(), "renaming to the same name must do nothing")
	assert.False(t, f.renamed.called)
}

func TestRenameBranch_RejectsEmptyBranch(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)

	_, err := f.uc.RenameBranch(context.Background(), "w1", "   ")

	require.Error(t, err)
	assert.Empty(t, f.git.ops())
}

// If the directory move fails after git has already renamed the branch, the
// branch rename must be undone — otherwise git and the record disagree, which
// is the exact divergence this whole feature exists to prevent.
func TestRenameBranch_RollsBackBranchWhenMoveFails(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	// Make the destination's parent a FILE so os.Rename cannot create the leaf.
	blocker := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo", "blocked")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	_, err := f.uc.RenameBranch(context.Background(), "w1", "blocked/x")

	require.Error(t, err)
	ops := f.git.ops()
	assert.Equal(t, 2, countOp(ops, "RenameBranch"),
		"the branch must be renamed and then renamed back")
	assert.Equal(t, "testing", f.git.lastRenameTo(), "rollback must restore the original name")
	assert.DirExists(t, filepath.Join(f.oldRoot, "worktree"), "the workspace must stay put")
	assert.False(t, f.renamed.called, "no record update when the move failed")
}

// A client that hangs up mid-rename must not be able to split git, the disk and
// the record apart. The endpoint is synchronous, so the request context reaches
// every git call through exec.CommandContext: on a disconnect the worktree
// repair dies AND so does the compensation that exists to undo it, leaving git
// on the new branch with the directory and the record still on the old one —
// a branch the record no longer names and later merges/removes cannot find.
//
// Once `git branch -m` has landed the operation must therefore finish or fully
// unwind on its own. Either end state is correct here; a split one is not.
func TestRenameBranch_ClientDisconnectLeavesNoSplitState(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The client hangs up the instant the branch rename lands.
	f.git.onBranchRenamed = cancel

	got, err := f.uc.RenameBranch(ctx, "w1", "feature/x")

	newRoot := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo", "feature", "x")
	if err != nil {
		assert.Equal(t, "testing", f.git.branchNow, "an aborted rename must restore the branch")
		assert.False(t, f.renamed.called, "an aborted rename must not touch the record")
		assert.DirExists(t, filepath.Join(f.oldRoot, "worktree"))
		assert.NoDirExists(t, newRoot)
		return
	}
	assert.Equal(t, "feature/x", f.git.branchNow)
	assert.Equal(t, "feature/x", f.renamed.branch, "the record must carry the completed rename")
	assert.Equal(t, "feature/x", got.Branch)
	assert.DirExists(t, filepath.Join(newRoot, "worktree"))
	assert.NoDirExists(t, f.oldRoot)
}

// A nested branch name (feature/x) nests its workspace root a directory deeper,
// so moving the root out leaves <slug>/feature behind — empty, invisible, and
// squatting the name "feature" forever: the next workspace on a branch actually
// CALLED feature is refused by the clash scan for a directory nothing occupies.
func TestRenameBranch_PrunesTheEmptyNestedParentItLeaves(t *testing.T) {
	f := newRenameFixture(t, "feature/x", nil)
	slugDir := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo")
	require.DirExists(t, filepath.Join(slugDir, "feature"))

	_, err := f.uc.RenameBranch(context.Background(), "w1", "cleanup")
	require.NoError(t, err)

	assert.NoDirExists(t, filepath.Join(slugDir, "feature"),
		"the emptied nested parent must not be left squatting the name")
	assert.DirExists(t, slugDir, "the prune must stop at the slug directory")
}

// The prune only ever removes directories it EMPTIED. A sibling workspace still
// living under the same nested parent keeps that parent alive.
func TestRenameBranch_KeepsTheNestedParentASiblingStillOccupies(t *testing.T) {
	f := newRenameFixture(t, "feature/x", nil)
	slugDir := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo")
	sibling := filepath.Join(slugDir, "feature", "y", "worktree")
	require.NoError(t, os.MkdirAll(sibling, 0o755))

	_, err := f.uc.RenameBranch(context.Background(), "w1", "cleanup")
	require.NoError(t, err)

	assert.DirExists(t, sibling, "a sibling under the same parent must be untouched")
}

func countOp(ops []string, want string) int {
	n := 0
	for _, op := range ops {
		if op == want {
			n++
		}
	}
	return n
}

// The record write is the LAST step of a rename, and it is the one step with no
// compensation: git has landed the new branch name and the whole workspace root
// has already moved when the aggregate refuses (validation, OCC exhaustion, a
// full command queue). Returning there leaves git and the disk on the new branch
// with the record still naming the old one — the same split state the detached
// context exists to prevent, reached by a different door. Every later
// merge/remove then looks for a branch that is no longer there.
func TestRenameBranch_RollsBackEverythingWhenTheRecordWriteFails(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	f.recordErr = errors.New("aggregate refused")

	_, err := f.uc.RenameBranch(context.Background(), "w1", "feature/x")

	require.Error(t, err)
	newRoot := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo", "feature", "x")
	assert.Equal(t, 2, countOp(f.git.ops(), "RenameBranch"),
		"the branch must be renamed and then renamed back")
	assert.Equal(t, "testing", f.git.lastRenameTo(), "git must be back on the original branch")
	assert.Equal(t, "testing", f.git.branchNow)
	assert.DirExists(t, filepath.Join(f.oldRoot, "worktree"),
		"the workspace root must be back where the record still points")
	assert.NoDirExists(t, newRoot, "nothing may be left at the abandoned destination")
}

// The unwind has to REBUILD what the completed relocation tore down: the move
// pruned the emptied nested parent of the old root, so moving back has to
// recreate it — and the nested parent the move CREATED at the destination has to
// go, or it squats the name exactly as the forward path's prune exists to
// prevent.
func TestRenameBranch_RecordWriteFailure_RestoresTheNestedLayout(t *testing.T) {
	f := newRenameFixture(t, "feature/x", nil)
	f.recordErr = errors.New("aggregate refused")
	slugDir := filepath.Join(f.home, "projects", "p1", "github.com", "test", "repo")

	_, err := f.uc.RenameBranch(context.Background(), "w1", "release/y")

	require.Error(t, err)
	assert.DirExists(t, filepath.Join(slugDir, "feature", "x", "worktree"),
		"the nested parent the forward move pruned must be recreated by the unwind")
	assert.NoDirExists(t, filepath.Join(slugDir, "release"),
		"the nested parent created for the abandoned destination must not squat the name")
}

// git's worktree registration was already repaired onto the new path by the time
// the record write failed, so the unwind has to point it back at the old one —
// otherwise the tree sits where the record says while git looks somewhere else.
func TestRenameBranch_RecordWriteFailure_RepairsGitBackOntoTheOldPath(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	f.recordErr = errors.New("aggregate refused")

	_, err := f.uc.RenameBranch(context.Background(), "w1", "feature/x")

	require.Error(t, err)
	repaired := f.git.repairedPaths()
	require.NotEmpty(t, repaired)
	assert.Equal(t, filepath.Join(f.oldRoot, "worktree"), repaired[len(repaired)-1],
		"the last repair must re-register the worktree at the path the record still names")
}

// repairedPaths returns the worktree path of every WorktreeRepair call, in order.
func (f *fakeGit) repairedPaths() []string {
	var paths []string
	for _, c := range f.calls {
		if c.op == "WorktreeRepair" {
			paths = append(paths, c.args[1])
		}
	}
	return paths
}
