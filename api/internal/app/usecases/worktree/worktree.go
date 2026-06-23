package worktree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// CreateChildInput carries the fields needed to create a worktree-backed child.
type CreateChildInput struct {
	RepoID       string
	ProjectID    string
	RepoPath     string
	RemoteURL    string // git remote origin URL; required for worktree path derivation
	Branch       string
	ParentID     string
	ParentBranch string
	// ForceLocked overrides the provider-driven locked check and marks the
	// workspace locked regardless of whether the branch is protected. Useful
	// when the caller knows the workspace should be immutable (e.g. the repo's
	// main branch workspace adopted without a git provider).
	ForceLocked bool
}

// Usecase orchestrates the worktree hierarchy (07): worktree-backed
// create, local child→parent merge (all three strategies), re-parenting, and
// cascade delete. It composes the git-engine primitives with 3A's Workspace
// Asynx commands; it forks neither. All hierarchy guards live here.
type Usecase interface {
	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (MergeResult, error)
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	RebaseOntoParent(
		ctx context.Context,
		childID string,
	) (domain.Workspace, error)
	DeleteCascade(
		ctx context.Context,
		rootID string,
	) error
	ReconcileAll(
		ctx context.Context,
	) error
}

type worktreeUsecase struct {
	workspaces  workspace.Workspace
	git         enginegit.Engine
	provider    engineprovider.Engine
	repos       store.Store[domain.Repository, string]
	now         func() time.Time
	crowbarHome func() (string, error)
}

// New builds the hierarchy usecase. repos resolves a workspace's repository main
// path (via RepoID) so worktree removal and branch deletion run against the repo,
// never against a child's own worktree (07 §5).
func New(
	workspaces workspace.Workspace,
	git enginegit.Engine,
	provider engineprovider.Engine,
	repos store.Store[domain.Repository, string],
	now func() time.Time,
	crowbarHome func() (string, error),
) Usecase {
	return &worktreeUsecase{
		workspaces:  workspaces,
		git:         git,
		provider:    provider,
		repos:       repos,
		now:         now,
		crowbarHome: crowbarHome,
	}
}

func (u *worktreeUsecase) CreateChild(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	// Repos with no on-disk path (e.g. virtual/test repos) skip all git
	// operations and create a workspace row directly.
	if in.RepoPath == "" {
		return u.workspaces.Create(ctx, workspace.CreateInput{
			ID:        uuid.NewString(),
			RepoID:    in.RepoID,
			ProjectID: in.ProjectID,
			Branch:    in.Branch,
			ParentID:  in.ParentID,
			Protected: in.ForceLocked,
		}, u.now())
	}
	// When ParentID is empty and the requested branch matches the parent branch
	// (i.e. the repo's default branch), the workspace IS the main worktree —
	// adopt the existing repo path instead of creating a new git worktree.
	if in.ParentID == "" && in.Branch == in.ParentBranch {
		return u.adoptMainWorktree(ctx, in)
	}
	wsID := uuid.NewString()
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: crowbar home: %w", err)
	}
	// Resolve the locked flag BEFORE creating the worktree so a provider hiccup
	// can't leave an orphaned worktree+branch on disk (it has no on-disk effect).
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: locked: %w", err)
	}
	path := worktreepath.For(home, in.ProjectID, in.RepoID, wsID)
	startSha, err := u.addWorktree(ctx, in, path)
	if err != nil {
		return domain.Workspace{}, err
	}
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           wsID,
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		WorktreePath: path,
		ForkPointSha: startSha,
		ParentID:     in.ParentID,
		Protected:    locked || in.ForceLocked,
	}, u.now())
	if err != nil {
		// The worktree + branch are on disk but the workspace row never landed.
		// Clean them up best-effort so a fresh-wsID retry isn't blocked by the
		// orphaned branch and the worktree dir doesn't dangle forever.
		if rmErr := u.git.WorktreeRemove(ctx, in.RepoPath, path); rmErr != nil {
			slog.WarnContext(ctx, "create child: cleanup worktree after failed create",
				"worktree", path, "err", rmErr)
		}
		if delErr := u.git.ForceDeleteBranch(ctx, in.RepoPath, in.Branch); delErr != nil {
			slog.WarnContext(ctx, "create child: cleanup branch after failed create",
				"branch", in.Branch, "err", delErr)
		}
		return domain.Workspace{}, err
	}
	return ws, nil
}

// addWorktree applies the spec-§3 checkout-vs-create decision and returns the
// fork-point SHA the workspace aggregate should record.
//
// If the requested branch already exists on the `origin` remote, it is fetched
// and checked out into a fresh worktree (tracking origin/<branch>); the fork
// point is the resolved tip of origin/<branch>. Otherwise the branch is created
// locally from ParentBranch, and the fork point is the start SHA reported by
// WorktreeAddBranch. The two paths have DIFFERENT fork-point semantics: the
// checkout path's fork point is the remote tip (so later merge/reparent math
// diffs against what the remote already contains), while the create path's fork
// point is exactly where the new branch diverged from its parent.
func (u *worktreeUsecase) addWorktree(
	ctx context.Context,
	in CreateChildInput,
	path string,
) (string, error) {
	exists, err := u.git.RemoteBranchExists(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return "", fmt.Errorf("create child: remote branch exists: %w", err)
	}
	if exists {
		return u.checkoutRemoteBranch(ctx, in, path)
	}
	startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, in.ParentBranch)
	if err != nil {
		return "", fmt.Errorf("create child: worktree add: %w", err)
	}
	return startSha, nil
}

// checkoutRemoteBranch fetches origin/<branch> and adds a worktree checking out
// the existing branch. The fork point is the resolved origin/<branch> tip.
func (u *worktreeUsecase) checkoutRemoteBranch(
	ctx context.Context,
	in CreateChildInput,
	path string,
) (string, error) {
	if err := u.git.FetchRef(ctx, in.RepoPath, in.Branch); err != nil {
		return "", fmt.Errorf("create child: fetch ref: %w", err)
	}
	forkPoint, err := u.git.RevParse(ctx, in.RepoPath, "origin/"+in.Branch)
	if err != nil {
		return "", fmt.Errorf("create child: resolve remote ref: %w", err)
	}
	if err := u.git.WorktreeAdd(ctx, in.RepoPath, path, in.Branch); err != nil {
		return "", fmt.Errorf("create child: worktree checkout: %w", err)
	}
	return forkPoint, nil
}

// adoptMainWorktree registers the repository's main worktree as a workspace
// without creating a new git worktree. It is used when the requested branch is
// the repository's default branch and there is no parent workspace.
func (u *worktreeUsecase) adoptMainWorktree(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	startSha, err := u.git.RevParse(ctx, in.RepoPath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: adopt main worktree: rev-parse HEAD: %w", err)
	}
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: adopt main worktree: locked: %w", err)
	}
	return u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		WorktreePath: in.RepoPath,
		ForkPointSha: startSha,
		ParentID:     in.ParentID,
		Protected:    locked || in.ForceLocked,
	}, u.now())
}

func (u *worktreeUsecase) resolveLocked(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	protected, err := u.provider.ProtectedBranches(ctx, repoPath)
	if err != nil {
		return false, err
	}
	for _, b := range protected {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}

func (u *worktreeUsecase) MergeIntoParent(
	ctx context.Context,
	childID string,
	strategy gitdomain.MergeStrategy,
) (MergeResult, error) {
	child, parent, err := u.loadMergePair(ctx, childID)
	if err != nil {
		return MergeResult{}, err
	}
	if guardErr := u.guardMerge(ctx, child, parent, strategy); guardErr != nil {
		return MergeResult{}, guardErr
	}
	if mergeErr := u.runMerge(ctx, child, parent, strategy); mergeErr != nil {
		return u.handleMergeError(ctx, child, parent, strategy, mergeErr)
	}
	return u.finalizeMerge(ctx, child, parent)
}

func (u *worktreeUsecase) loadMergePair(
	ctx context.Context,
	childID string,
) (domain.Workspace, domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, domain.Workspace{}, fmt.Errorf("merge: get child: %w", err)
	}
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if err != nil {
		return domain.Workspace{}, domain.Workspace{}, fmt.Errorf("merge: get parent: %w", err)
	}
	return child, parent, nil
}

func (u *worktreeUsecase) guardMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
) error {
	if parent.Status == domain.WorkspaceStatusLocked {
		return ErrParentLocked
	}
	if strategy != gitdomain.MergeStrategyRebase {
		return nil
	}
	hasKids, err := u.childHasChildren(ctx, child.ID)
	if err != nil {
		return fmt.Errorf("merge: leaf check: %w", err)
	}
	if hasKids {
		return ErrRebaseNonLeaf
	}
	return nil
}

func (u *worktreeUsecase) runMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
) error {
	if strategy == gitdomain.MergeStrategySquash {
		subject := fmt.Sprintf("Squash merge %s", child.Branch)
		return u.git.MergeSquash(ctx, parent.WorktreePath, child.Branch, subject)
	}
	if strategy == gitdomain.MergeStrategyRebase {
		return u.runRebaseMerge(ctx, child, parent)
	}
	return u.git.Merge(ctx, parent.WorktreePath, child.Branch)
}

func (u *worktreeUsecase) runRebaseMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
) error {
	return u.git.RebaseThenFFMerge(
		ctx,
		child.WorktreePath,
		parent.Branch,
		parent.WorktreePath,
		child.Branch,
	)
}

func (u *worktreeUsecase) handleMergeError(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
	mergeErr error,
) (MergeResult, error) {
	if !errors.Is(mergeErr, enginegit.ErrConflict) {
		return MergeResult{}, fmt.Errorf("merge: run: %w", mergeErr)
	}
	// Try-then-warn (consistent with replayAndReparent): a conflicting
	// merge-into-parent must NEVER leave a worktree stuck. The merge runs in the
	// PARENT for the squash/plain-merge strategies and in the CHILD for the
	// rebase strategy, so abort the in-progress op in whichever worktree holds
	// it. The conflict is then surfaced only as the child's pr-conflicts state;
	// the user resolves it via "Rebase onto parent" (which keeps a resolvable
	// rebase in the child's OWN worktree) and re-runs the merge once clean.
	abortPath := parent.WorktreePath
	if strategy == gitdomain.MergeStrategyRebase {
		abortPath = child.WorktreePath
	}
	if abortErr := u.git.OperationAbort(ctx, abortPath); abortErr != nil {
		// Best-effort: do not fail the whole operation, but log so a worktree
		// that somehow stayed stuck is visible rather than silently bricked.
		slog.WarnContext(ctx, "merge: abort after conflict failed; worktree may be stuck",
			"workspace_id", child.ID, "abort_path", abortPath, "err", abortErr)
	}
	// A local merge/rebase conflict transitions the child to Status=pr-conflicts
	// (07 §3.1, 00 §6.1): the HasConflicts sync input drives the status enum.
	_, err := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           child.ID,
		HasConflicts: true,
	}, u.now())
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge: set conflicts: %w", err)
	}
	return MergeResult{ConflictsPending: true}, nil
}

func (u *worktreeUsecase) finalizeMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
) (MergeResult, error) {
	tip, err := u.git.RevParse(ctx, parent.WorktreePath, "HEAD")
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge: parent tip: %w", err)
	}
	if _, err := u.workspaces.UpdateForkPoint(ctx, child.ID, tip); err != nil {
		return MergeResult{}, fmt.Errorf("merge: update fork point: %w", err)
	}
	if _, err := u.resyncSummary(ctx, parent.ID, parent.WorktreePath, parent.ForkPointSha); err != nil {
		slog.WarnContext(ctx, "merge: parent summary resync failed; read-model will self-correct", "workspace_id", parent.ID, "err", err)
	}
	if _, err := u.resyncSummary(ctx, child.ID, child.WorktreePath, tip); err != nil {
		slog.WarnContext(ctx, "merge: child summary resync failed; read-model will self-correct", "workspace_id", child.ID, "err", err)
	}
	return MergeResult{ParentTipSha: tip}, nil
}

// resyncSummary recomputes a workspace's working-tree summary from git and
// pushes it into the read model so Added/Deleted/HasCommits/HasConflicts reflect
// the post-merge state of both the parent and the kept child. It returns the
// hasConflicts it computed so callers (e.g. ReconcileAll) can act on it without a
// second WorkingTreeSummary call.
func (u *worktreeUsecase) resyncSummary(
	ctx context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (bool, error) {
	added, deleted, hasConflicts, hasCommits, err := u.git.WorkingTreeSummary(ctx, worktreePath, forkPointSha)
	if err != nil {
		return false, fmt.Errorf("merge: resync summary: %w", err)
	}
	_, err = u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           id,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	}, u.now())
	if err != nil {
		return false, fmt.Errorf("merge: resync summary: sync: %w", err)
	}
	return hasConflicts, nil
}

func (u *worktreeUsecase) Reparent(
	ctx context.Context,
	childID string,
	newParentID string,
) (domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get child: %w", err)
	}
	newParent, err := u.workspaces.Get(ctx, newParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get new parent: %w", err)
	}
	if guardErr := u.guardReparent(ctx, child, newParent); guardErr != nil {
		return domain.Workspace{}, guardErr
	}
	tip, err := u.git.RevParse(ctx, newParent.WorktreePath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: new parent tip: %w", err)
	}
	return u.replayAndReparent(ctx, child, newParentID, tip)
}

// replayAndReparent tries to rebase the child onto the new parent's tip, then
// settles the move. Try-then-warn: a CLEAN rebase moves the child onto the new
// parent and integrates it; a CONFLICT never leaves the rebase stuck — it is
// ABORTED back to a clean worktree, yet the child is STILL moved under the new
// parent ("moved on paper, not yet integrated"). The predicted-conflict overlay
// (mergeConflicts) marks it; the user finishes the rebase on their own terms.
func (u *worktreeUsecase) replayAndReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	tip string,
) (domain.Workspace, error) {
	rebaseErr := u.git.RebaseOnto(ctx, child.WorktreePath, tip, child.ForkPointSha, child.Branch)
	if rebaseErr == nil {
		// Clean: the branch is genuinely based on the new parent's tip now.
		return u.settleReparent(ctx, child, newParentID, tip)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("reparent: rebase onto: %w", rebaseErr)
	}
	// Conflict: abort the rebase so the worktree returns to clean (never the stuck
	// mid-rebase mess), but keep the move.
	if abortErr := u.git.OperationAbort(ctx, child.WorktreePath); abortErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: abort after conflict: %w", abortErr)
	}
	// Fork point = merge-base of the branch and the new parent, so diffs/summaries
	// read against the new parent's lineage until a clean rebase finalizes it.
	base, baseErr := u.git.MergeBase(ctx, child.WorktreePath, tip, child.Branch)
	if baseErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: merge-base: %w", baseErr)
	}
	return u.settleReparent(ctx, child, newParentID, base)
}

// settleReparent persists the child's new parent + fork point and resyncs its
// working-tree summary (best-effort, like finalizeMerge) so a prior conflict
// status clears and the diff stats reflect the new base.
func (u *worktreeUsecase) settleReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	forkPoint string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Reparent(ctx, child.ID, newParentID, forkPoint, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: persist: %w", err)
	}
	if _, syncErr := u.resyncSummary(ctx, child.ID, child.WorktreePath, forkPoint); syncErr != nil {
		slog.WarnContext(ctx, "reparent: child summary resync failed; read-model will self-correct",
			"workspace_id", child.ID, "err", syncErr)
	}
	return ws, nil
}

// RebaseOntoParent is the user-initiated "finish the move" action for a
// moved-but-conflicting child: it rebases the child onto its CURRENT parent's
// tip. Unlike the automatic reparent (which aborts on conflict), this KEEPS a
// conflicting rebase in progress so the user can resolve it with the standard
// conflict tooling; a clean rebase integrates the child. The intended fork point
// (the parent tip) is persisted up front so the branch reads correctly once
// resolved.
func (u *worktreeUsecase) RebaseOntoParent(
	ctx context.Context,
	childID string,
) (domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: get child: %w", err)
	}
	if child.ParentID == "" {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: workspace has no parent")
	}
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: get parent: %w", err)
	}
	tip, err := u.git.RevParse(ctx, parent.WorktreePath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: parent tip: %w", err)
	}
	rebaseErr := u.git.RebaseOnto(ctx, child.WorktreePath, tip, child.ForkPointSha, child.Branch)
	if rebaseErr == nil {
		// Clean: the child is genuinely integrated onto the parent.
		return u.settleReparent(ctx, child, child.ParentID, tip)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: %w", rebaseErr)
	}
	// Conflict: KEEP the rebase in progress (the user resolves it with the
	// standard conflict tooling). Persist the intended fork point now so the
	// branch reads correctly once resolved, and surface the conflict via status.
	if _, err := u.workspaces.Reparent(ctx, child.ID, child.ParentID, tip, u.now()); err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: persist: %w", err)
	}
	ws, err := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           child.ID,
		HasConflicts: true,
	}, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: set conflicts: %w", err)
	}
	return ws, nil
}

func (u *worktreeUsecase) guardReparent(
	ctx context.Context,
	child domain.Workspace,
	newParent domain.Workspace,
) error {
	if child.ID == newParent.ID {
		return ErrSelfParent
	}
	if newParent.Status == domain.WorkspaceStatusLocked {
		return ErrNewParentLocked
	}
	hasKids, err := u.childHasChildren(ctx, child.ID)
	if err != nil {
		return fmt.Errorf("reparent: leaf check: %w", err)
	}
	if hasKids {
		return ErrChildHasChildren
	}
	return nil
}

func (u *worktreeUsecase) DeleteCascade(
	ctx context.Context,
	rootID string,
) error {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("delete cascade: list: %w", err)
	}
	index := indexByID(all)
	root, ok := index[rootID]
	if !ok {
		return fmt.Errorf("delete cascade: workspace %s: %w", rootID, apperr.ErrNotFound)
	}
	if root.Status == domain.WorkspaceStatusLocked {
		return ErrWorkspaceLocked
	}
	order := cascade.Plan(rootID, nodesFrom(all))
	for _, id := range order {
		if removeErr := u.removeOne(ctx, index[id]); removeErr != nil {
			return fmt.Errorf("delete cascade: remove %s: %w", id, removeErr)
		}
	}
	return nil
}

func (u *worktreeUsecase) removeOne(
	ctx context.Context,
	ws domain.Workspace,
) error {
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		// Can't resolve the repo path — still drop the read-model row so the cascade
		// doesn't leave a ghost workspace pointing at an unreachable worktree.
		slog.WarnContext(ctx, "cascade: repo path unresolved; dropping row best-effort",
			"ws", ws.ID, "err", err)
		return u.workspaces.Delete(ctx, ws.ID)
	}
	// Best-effort git teardown: a failure here (branch checked out elsewhere, a
	// transient index lock, an already-removed worktree) must NOT abort the cascade
	// or leave a GHOST row pointing at a gone worktree — that breaks every future op
	// on it and makes a re-run cascade fail too. Log and continue; the row is always
	// dropped, and an orphaned worktree on disk is reaped by `git worktree prune`.
	if removeErr := u.git.WorktreeRemove(ctx, repoPath, ws.WorktreePath); removeErr != nil {
		slog.WarnContext(ctx, "cascade: worktree remove failed (continuing)",
			"ws", ws.ID, "worktree", ws.WorktreePath, "err", removeErr)
	}
	if delErr := u.git.ForceDeleteBranch(ctx, repoPath, ws.Branch); delErr != nil {
		slog.WarnContext(ctx, "cascade: branch delete failed (continuing)",
			"ws", ws.ID, "branch", ws.Branch, "err", delErr)
	}
	return u.workspaces.Delete(ctx, ws.ID)
}

func (u *worktreeUsecase) repoPathFor(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	repo, err := u.repos.FindByKey(ctx, ws.RepoID)
	if err != nil {
		return "", fmt.Errorf("repo path: %w", err)
	}
	if repo == nil {
		return "", fmt.Errorf("worktree: repo %s not found", ws.RepoID)
	}
	return repo.Path, nil
}

// ReconcileAll is the one-shot startup recovery sweep (H19). After a crash or
// restart the persisted per-workspace badge (Added/Deleted, pr-conflicts) is
// whatever was last written and otherwise only self-corrects on the next
// fsnotify event in that workspace — so a conflict resolved out-of-band stays
// pr-conflicts, stale diff counts linger, and worktrees orphaned mid-teardown
// are never reaped. This re-syncs each workspace's working-tree summary from
// real git, clears a now-stale pr-conflicts, and prunes orphaned worktrees per
// repo. It is best-effort throughout: a failure on one workspace or repo is
// logged and skipped, never aborting the sweep, and ReconcileAll returns nil so
// boot is never failed by recovery work.
func (u *worktreeUsecase) ReconcileAll(
	ctx context.Context,
) error {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		// Listing is the only catastrophic failure: without the rows there is
		// nothing to reconcile. Log and return nil — never fail boot.
		slog.WarnContext(ctx, "recovery sweep: list workspaces failed; skipping", "err", err)
		return nil
	}
	for _, ws := range all {
		u.reconcileOne(ctx, ws)
	}
	u.pruneRepos(ctx, all)
	return nil
}

// reconcileOne re-syncs a single workspace's working-tree summary from real git
// and clears a stale pr-conflicts. Workspaces with no on-disk worktree (virtual
// / no-path rows) are skipped — there is nothing to read from disk.
func (u *worktreeUsecase) reconcileOne(
	ctx context.Context,
	ws domain.Workspace,
) {
	if ws.WorktreePath == "" {
		return
	}
	// resyncSummary refreshes Added/Deleted/HasCommits and sets pr-conflicts when
	// there are real conflict markers; it returns the hasConflicts it computed so
	// we can clear a now-stale pr-conflicts without a second WorkingTreeSummary.
	hasConflicts, err := u.resyncSummary(ctx, ws.ID, ws.WorktreePath, ws.ForkPointSha)
	if err != nil {
		slog.WarnContext(ctx, "recovery sweep: summary resync failed; skipping workspace",
			"workspace_id", ws.ID, "worktree", ws.WorktreePath, "err", err)
		return
	}
	if hasConflicts {
		return
	}
	// No real conflict on disk: clear a sticky pr-conflicts (a no-op when the
	// status isn't pr-conflicts) so a workspace whose conflict was resolved while
	// the daemon was down drops its stale warning.
	if _, err := u.workspaces.ResolveConflicts(ctx, ws.ID, u.now()); err != nil {
		slog.WarnContext(ctx, "recovery sweep: resolve stale conflicts failed",
			"workspace_id", ws.ID, "err", err)
	}
}

// pruneRepos reaps orphaned worktree registrations for each DISTINCT repo among
// the workspaces, so a worktree whose directory vanished mid-teardown (crash) no
// longer blocks future ops. Best-effort: a repo-path resolution or prune failure
// is logged and skipped.
func (u *worktreeUsecase) pruneRepos(
	ctx context.Context,
	all []domain.Workspace,
) {
	pruned := make(map[string]struct{})
	for _, ws := range all {
		repoPath, err := u.repoPathFor(ctx, ws)
		if err != nil {
			slog.WarnContext(ctx, "recovery sweep: repo path unresolved; skipping prune",
				"workspace_id", ws.ID, "repo_id", ws.RepoID, "err", err)
			continue
		}
		if _, done := pruned[repoPath]; done {
			continue
		}
		pruned[repoPath] = struct{}{}
		if err := u.git.WorktreePrune(ctx, repoPath); err != nil {
			slog.WarnContext(ctx, "recovery sweep: worktree prune failed",
				"repo_path", repoPath, "err", err)
		}
	}
}

func (u *worktreeUsecase) childHasChildren(
	ctx context.Context,
	childID string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, err
	}
	for _, ws := range all {
		// A self-loop (ws.ParentID == ws.ID) must not count the node as its own
		// child, or a corrupted self-parented workspace becomes unreparentable.
		if ws.ParentID == childID && ws.ID != childID {
			return true, nil
		}
	}
	return false, nil
}

func nodesFrom(
	all []domain.Workspace,
) []cascade.Node {
	nodes := make([]cascade.Node, 0, len(all))
	for _, ws := range all {
		nodes = append(nodes, cascade.Node{
			ID:     ws.ID,
			Parent: ws.ParentID,
			Locked: ws.Status == domain.WorkspaceStatusLocked,
		})
	}
	return nodes
}

func indexByID(
	all []domain.Workspace,
) map[string]domain.Workspace {
	index := make(map[string]domain.Workspace, len(all))
	for _, ws := range all {
		index[ws.ID] = ws
	}
	return index
}
