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
		id, branch string
		called     bool
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
		RenameBranchFn: func(id, b string) (domain.Workspace, error) {
			f.renamed.called = true
			f.renamed.id, f.renamed.branch = id, b
			if f.recordErr != nil {
				return domain.Workspace{}, f.recordErr
			}
			return domain.Workspace{ID: id, Branch: b, WorktreePath: self.WorktreePath}, nil
		},
	}
	f.uc = worktree.New(ws, f.git, &fakeProvider{},
		&fakeRepoStore{path: "/repo", remoteURL: "https://github.com/test/repo.git"},
		newNow(), func() (string, error) { return home, nil })
	return f
}

// TestRenameBranch_RenamesTheRefAndLeavesTheWorkspaceWhereItIs is the whole
// operation now: git moves, the record follows, the disk does not.
func TestRenameBranch_RenamesTheRefAndLeavesTheWorkspaceWhereItIs(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)

	got, err := f.uc.RenameBranch(context.Background(), "w1", "feature/x")
	require.NoError(t, err)

	assert.Equal(t, "feature/x", f.git.lastRenameTo(), "git must rename the ref")
	assert.True(t, f.renamed.called, "the record must be updated")
	assert.Equal(t, "feature/x", f.renamed.branch)
	assert.Equal(t, filepath.Join(f.oldRoot, "worktree"), got.WorktreePath,
		"the recorded path must not change")
	assert.DirExists(t, filepath.Join(f.oldRoot, "worktree"),
		"the workspace root must stay exactly where it was")
	assert.NotContains(t, f.git.ops(), "WorktreeRepair",
		"nothing moved, so git needs no repair")
}

// TestRegression_RenameBranch_IntoItsOwnNamespace pins a rename the old
// move-the-directory implementation could not express at all: testing →
// testing/x made the destination a child of the source, and the kernel refuses
// that with EINVAL.
func TestRegression_RenameBranch_IntoItsOwnNamespace(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)

	_, err := f.uc.RenameBranch(context.Background(), "w1", "testing/x")

	require.NoError(t, err)
	assert.Equal(t, "testing/x", f.git.lastRenameTo())
	assert.DirExists(t, filepath.Join(f.oldRoot, "worktree"))
}

// TestRenameBranch_LeavesTheAgentChatsWhereTheyAre: the chats tree is a sibling
// of the worktree inside the root, and the root does not move.
func TestRenameBranch_LeavesTheAgentChatsWhereTheyAre(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	ledger := filepath.Join(f.oldRoot, "chats", "c1", "ledger")
	require.NoError(t, os.MkdirAll(ledger, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ledger, "1.turn"), []byte("hi"), 0o600))

	_, err := f.uc.RenameBranch(context.Background(), "w1", "renamed")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(ledger, "1.turn"),
		"a rename must not disturb the conversation ledger")
}

func TestRenameBranch_RejectsLockedWorkspaceBeforeTouchingGit(t *testing.T) {
	locked := domain.Workspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "testing",
		WorktreePath: "/anything/worktree", Status: domain.WorkspaceStatusLocked,
	}
	f := newRenameFixture(t, "testing", []domain.Workspace{locked})

	_, err := f.uc.RenameBranch(context.Background(), "w1", "renamed")

	require.ErrorIs(t, err, worktree.ErrWorkspaceLocked)
	assert.Empty(t, f.git.lastRenameTo(), "git must not be touched")
	assert.False(t, f.renamed.called)
}

// An adopted checkout is the user's own repository. The rename no longer moves
// anything, but it would still rewrite a ref inside a directory Crowbar did not
// create, and that stays theirs to do.
func TestRenameBranch_RejectsWorkspaceOutsideCrowbarHome(t *testing.T) {
	outside := domain.Workspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "testing",
		WorktreePath: "/Users/someone/Projects/repo", Status: domain.WorkspaceStatusNew,
	}
	f := newRenameFixture(t, "testing", []domain.Workspace{outside})

	_, err := f.uc.RenameBranch(context.Background(), "w1", "renamed")

	require.ErrorIs(t, err, worktree.ErrRenameUnmanagedWorkspace)
	assert.Empty(t, f.git.lastRenameTo(), "git must not be touched")
}

func TestRenameBranch_RejectsBranchAnotherWorkspaceHolds(t *testing.T) {
	self := domain.Workspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "testing",
		Status: domain.WorkspaceStatusNew,
	}
	other := domain.Workspace{
		ID: "w2", RepoID: "r1", ProjectID: "p1", Branch: "taken",
		Status: domain.WorkspaceStatusNew,
	}
	f := newRenameFixture(t, "testing", nil)
	self.WorktreePath = filepath.Join(f.oldRoot, "worktree")
	other.WorktreePath = filepath.Join(f.home, "projects", "p1", "other", "worktree")
	f = newRenameFixture(t, "testing", []domain.Workspace{self, other})

	_, err := f.uc.RenameBranch(context.Background(), "w1", "taken")

	require.ErrorIs(t, err, worktree.ErrBranchWorkspaceExists)
	assert.Empty(t, f.git.lastRenameTo(), "git must not be touched")
}

func TestRenameBranch_SameNameIsANoOp(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)

	_, err := f.uc.RenameBranch(context.Background(), "w1", "testing")

	require.NoError(t, err)
	assert.Empty(t, f.git.lastRenameTo(), "git must not be touched")
	assert.False(t, f.renamed.called)
}

func TestRenameBranch_RejectsEmptyBranch(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)

	_, err := f.uc.RenameBranch(context.Background(), "w1", "   ")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Empty(t, f.git.lastRenameTo())
}

// A refused record write leaves git on the new branch and the record on the old
// one. Nothing has to be unwound: the branch is a name, not a location, and no
// tree is resolved from it.
func TestRenameBranch_RecordWriteFailureSurfacesAndMovesNothing(t *testing.T) {
	f := newRenameFixture(t, "testing", nil)
	f.recordErr = errors.New("aggregate refused")

	_, err := f.uc.RenameBranch(context.Background(), "w1", "renamed")

	require.Error(t, err)
	assert.DirExists(t, filepath.Join(f.oldRoot, "worktree"),
		"the workspace must be untouched whatever the record does")
}

// countOp counts how many times op appears in a fakeGit call log.
func countOp(ops []string, want string) int {
	n := 0
	for _, op := range ops {
		if op == want {
			n++
		}
	}
	return n
}
