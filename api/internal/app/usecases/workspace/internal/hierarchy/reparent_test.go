package hierarchy_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestReparent_ConflictAbortsAndSettlesAtMergeBase(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseSha: "mergebase-sha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "OperationAbort", "MergeBase", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/cw"}, g.calls[3].args, "the abort must run in the CHILD worktree")
	assert.Equal(t, []string{"/cw", "ntip", "feat"}, g.calls[4].args, "merge-base of the new parent tip and the branch")
	assert.Equal(t, "c", rID)
	assert.Equal(t, "np", rParent)
	assert.Equal(t, "mergebase-sha", rSha, "the move lands at the MERGE-BASE, not the new parent's raw tip")
}

func TestReparent_ConflictAbortFails(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	persisted := false
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		persisted = true
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, operationAbortErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
	assert.False(t, persisted, "a failed abort must never persist a partial move")
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "OperationAbort"}, g.ops(),
		"no merge-base attempt after a failed abort")
}

func TestReparent_ConflictMergeBaseFails(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	persisted := false
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		persisted = true
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
	assert.False(t, persisted, "no move is persisted when the merge-base cannot be resolved")
}

func TestReparent_PersistError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, _, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_ResyncSummaryError_IsBestEffort(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	g := &fakeGit{revParseSha: "ntip"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	out, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err, "a resync sync-write failure must not fail the reparent")
	assert.Equal(t, "c", out.ID)
}

func TestRegression_ConflictingReparentMarksTheChildConflicting(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	var synced []workspace.SyncInput
	ws.SyncFn = func(_ context.Context, in workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		synced = append(synced, in)
		return domain.Workspace{ID: in.ID, Status: domain.WorkspaceStatusPRConflicts}, nil
	}
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: enginegit.ErrConflict, mergeBaseSha: "mb"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	out, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	require.Len(t, synced, 1)
	assert.True(t, synced[0].HasConflicts, "git shows no conflict after the abort; the move must")
	assert.Equal(t, domain.WorkspaceStatusPRConflicts, out.Status)

	ws.SyncFn = func(_ context.Context, _ workspace.SyncInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errBoom
	}
	_, err = uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom, "an unflagged conflicting move would be silent")
	assert.ErrorContains(t, err, "moved under np", "the move already landed; the failure must say so")
}

func TestRegression_ReparentKeepsCommitsAStaleForkPointWouldDrop(t *testing.T) {
	h := newRealUsecase(t)
	ctx := context.Background()
	newParent := h.createChild(t, "feature/a", h.parentID, h.baseBranch)
	commitInWorktree(t, newParent.WorktreePath, "a.txt", "a\n", "a work")
	child := h.createChild(t, "feature/b", h.parentID, h.baseBranch)
	commitInWorktree(t, child.WorktreePath, "b.txt", "b\n", "b work")
	_, err := h.workspaces.UpdateForkPoint(ctx, child.ID, revParse(t, child.WorktreePath, "HEAD"))
	require.NoError(t, err)
	h.quiesce()

	_, err = h.uc.Reparent(ctx, child.ID, newParent.ID)
	require.NoError(t, err)

	assert.Equal(t, "b work", trimNewline(gitRun(t, child.WorktreePath, "log", "--format=%s", "feature/a..HEAD")),
		"the branch keeps its own commit, replayed onto the new parent")
	assert.Empty(t, gitRun(t, child.WorktreePath, "merge-base", "--is-ancestor", "feature/a", "HEAD"))
}

func TestReparent_RebasesOntoNewTipAndUpdatesAggregate(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	var rID, rParent, rSha string
	ws.ReparentFn = func(_ context.Context, id, parentID, forkPointSha string, _ time.Time) (domain.Workspace, error) {
		rID, rParent, rSha = id, parentID, forkPointSha
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", mergeBaseSha: "base"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"RevParse", "MergeBase", "RebaseOnto", "WorkingTreeSummary"}, g.ops())
	assert.Equal(t, []string{"/np", "HEAD"}, g.calls[0].args)
	assert.Equal(t, []string{"/cw", "ntip", "feat"}, g.calls[1].args,
		"a child with no parent row replays from its merge-base with the new tip")
	assert.Equal(t, []string{"/cw", "ntip", "base", "feat"}, g.calls[2].args,
		"the replay starts at the merge-base, never the recorded fork point")
	assert.Equal(t, []string{"/cw", "ntip"}, g.calls[3].args)
	assert.Equal(t, "c", rID)
	assert.Equal(t, "np", rParent)
	assert.Equal(t, "ntip", rSha)
}

func TestReparent_RebaseOntoError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "fork", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	g := &fakeGit{revParseSha: "ntip", rebaseOnto: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func stacked(t *testing.T, h *realHarness) (domain.Workspace, domain.Workspace) {
	t.Helper()
	p := h.createChild(t, "feature/p", h.parentID, h.baseBranch)
	commitInWorktree(t, p.WorktreePath, "p.txt", "p\n", "p work")
	c := h.createChild(t, "feature/c", p.ID, p.Branch)
	commitInWorktree(t, c.WorktreePath, "c.txt", "c\n", "c work")
	return p, c
}

func amend(t *testing.T, p domain.Workspace) {
	t.Helper()
	writeFile(t, filepath.Join(p.WorktreePath, "p.txt"), "p rewritten\n")
	gitRun(t, p.WorktreePath, "commit", "-a", "--amend", "-m", "p work, amended")
}

func movedOnto(t *testing.T, h *realHarness, c domain.Workspace) string {
	t.Helper()
	q := h.createChild(t, "feature/q", h.parentID, h.baseBranch)
	commitInWorktree(t, q.WorktreePath, "q.txt", "q\n", "q work")
	_, err := h.uc.Reparent(context.Background(), c.ID, q.ID)
	require.NoError(t, err)
	return trimNewline(gitRun(t, c.WorktreePath, "log", "--format=%s", "feature/q..HEAD"))
}

func TestRegression_ReparentReplaysOnlyCommitsTheOldParentLacks(t *testing.T) {
	h := newRealUsecase(t)
	_, c := stacked(t, h)
	assert.Equal(t, "c work", movedOnto(t, h, c))
}

func TestRegression_ReparentOffAnAmendedParentCarriesItsOldCommit(t *testing.T) {
	h := newRealUsecase(t)
	p, c := stacked(t, h)
	amend(t, p)
	assert.Equal(t, "c work\np work", movedOnto(t, h, c))
}

func TestRegression_ReparentMovesAChildWhoseParentRowIsGone(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "gone", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	ws := reparentWS(child, newParent, nil)
	get := ws.GetFn
	ws.GetFn = func(ctx context.Context, id string) (domain.Workspace, error) {
		if id == "gone" {
			return domain.Workspace{}, fmt.Errorf("workspace: get: %w", asynxModels.ErrNotFound)
		}
		return get(ctx, id)
	}
	ws.ReparentFn = func(_ context.Context, id, _, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: id}, nil
	}
	g := &fakeGit{revParseSha: "ntip", mergeBaseSha: "base"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.Reparent(context.Background(), "c", "np")
	require.NoError(t, err)
	assert.Equal(t, []string{"/cw", "ntip", "feat"}, g.calls[1].args, "replays from the new tip")
}

func absorbedThenDropped(t *testing.T, h *realHarness) domain.Workspace {
	t.Helper()
	p, c := stacked(t, h)
	commitInWorktree(t, c.WorktreePath, "c2.txt", "c2\n", "c2 work")
	gitRun(t, p.WorktreePath, "merge", "--ff-only", "feature/c")
	gitRun(t, p.WorktreePath, "reset", "--hard", "HEAD~2")
	commitInWorktree(t, c.WorktreePath, "c3.txt", "c3\n", "c3 work")
	return c
}

func importedThenPruned(t *testing.T, h *realHarness) domain.Workspace {
	t.Helper()
	gitRun(t, h.repoPath, "checkout", "-b", "feature/c")
	commitInWorktree(t, h.repoPath, "c1.txt", "c1\n", "c work")
	commitInWorktree(t, h.repoPath, "c2.txt", "c2\n", "c2 work")
	gitRun(t, h.repoPath, "push", "origin", "feature/c")
	gitRun(t, h.repoPath, "checkout", h.baseBranch)
	gitRun(t, h.repoPath, "branch", "-D", "feature/c")
	p := h.createChild(t, "feature/p", h.parentID, h.baseBranch)
	c := h.createChild(t, "feature/c", p.ID, p.Branch)
	gitRun(t, p.WorktreePath, "merge", "--ff-only", "feature/c")
	gitRun(t, p.WorktreePath, "reset", "--hard", "HEAD@{1}")
	commitInWorktree(t, p.WorktreePath, "p.txt", "p\n", "p work")
	commitInWorktree(t, c.WorktreePath, "c3.txt", "c3\n", "c3 work")
	gitRun(t, h.repoPath, "push", "origin", "--delete", "feature/c")
	return c
}

func reflogTruncated(t *testing.T, h *realHarness) domain.Workspace {
	t.Helper()
	c := absorbedThenDropped(t, h)
	entries := strings.Fields(gitRun(t, h.repoPath, "reflog", "show", "--format=%h", "refs/heads/feature/c", "--"))
	for i := len(entries) - 1; i >= len(entries)-2; i-- {
		gitRun(t, h.repoPath, "reflog", "delete", "refs/heads/feature/c@{"+strconv.Itoa(i)+"}")
	}
	return c
}

func TestRegression_RestackNeverLosesAChildCommit(t *testing.T) {
	histories := map[string]func(*testing.T, *realHarness) domain.Workspace{
		"P absorbed C's commits and dropped them": absorbedThenDropped,
		"C was imported and origin's copy pruned": importedThenPruned,
		"C's reflog no longer starts where C did": reflogTruncated,
		"P was amended": func(t *testing.T, h *realHarness) domain.Workspace {
			t.Helper()
			p, c := stacked(t, h)
			amend(t, p)
			return c
		},
	}
	verbs := map[string]func(*testing.T, *realHarness, domain.Workspace){
		"reparent": func(t *testing.T, h *realHarness, c domain.Workspace) {
			t.Helper()
			movedOnto(t, h, c)
		},
		"rebase onto parent": func(t *testing.T, h *realHarness, c domain.Workspace) {
			t.Helper()
			_, err := h.uc.RebaseOntoParent(context.Background(), c.ID)
			require.NoError(t, err)
		},
	}
	for history, build := range histories {
		for verb, run := range verbs {
			t.Run(history+"/"+verb, func(t *testing.T) {
				h := newRealUsecase(t)
				c := build(t, h)
				h.quiesce()
				own := strings.Split(trimNewline(gitRun(t, h.repoPath, "log", "--format=%s", "feature/p..feature/c")), "\n")
				run(t, h, c)
				after := gitRun(t, h.repoPath, "log", "--format=%s", "feature/c")
				for _, subject := range own {
					assert.Contains(t, after, subject)
				}
			})
		}
	}
}

func TestReparent_GetChildError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestReparent_GetNewParentError(t *testing.T) {
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return domain.Workspace{ID: "c"}, nil
			}
			return domain.Workspace{}, errBoom
		},
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func reparentWS(
	child domain.Workspace,
	newParent domain.Workspace,
	children []domain.Workspace,
) *fakeWorkspace {
	list := append([]domain.Workspace{child, newParent}, children...)
	return &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == child.ID {
				return child, nil
			}
			if id == newParent.ID {
				return newParent, nil
			}
			return domain.Workspace{}, fmt.Errorf("not found: %s", id)
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return list, nil
		},
	}
}

func TestReparent_RevParseError(t *testing.T) {
	child := domain.Workspace{ID: "c", Branch: "feat", WorktreePath: "/cw", Provisioning: domain.WorkspaceProvisioned}
	newParent := domain.Workspace{ID: "np", WorktreePath: "/np", Provisioning: domain.WorkspaceProvisioned}
	g := &fakeGit{revParseErr: errBoom}
	uc := hierarchy.New(reparentWS(child, newParent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, errBoom)
}

func TestRebaseOntoParent_RejectsUnprovisionedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "f", Provisioning: domain.WorkspaceProvisioned}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, Provisioning: domain.WorkspacePlaceholder}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return parent, nil
		},
	}
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, hierarchy.ErrParentUnprovisioned)
	assert.Empty(t, g.calls)
}
