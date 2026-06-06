package worktree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
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
	Branch       string
	ParentID     string
	ParentBranch string
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
	DeleteCascade(
		ctx context.Context,
		rootID string,
	) error
}

type worktreeUsecase struct {
	workspaces workspace.Workspace
	git        enginegit.Engine
	provider   engineprovider.Engine
	repos      store.Store[domain.Repository, string]
	now        func() time.Time
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
) Usecase {
	return &worktreeUsecase{
		workspaces: workspaces,
		git:        git,
		provider:   provider,
		repos:      repos,
		now:        now,
	}
}

func (u *worktreeUsecase) CreateChild(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	path := worktreepath.For(in.RepoPath, in.Branch)
	startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, in.ParentBranch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: worktree add: %w", err)
	}
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: locked: %w", err)
	}
	return u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		WorktreePath: path,
		ForkPointSha: startSha,
		ParentID:     in.ParentID,
		Locked:       locked,
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
	if parent.Locked {
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
	if _, err := u.workspaces.SetPendingMerge(ctx, child.ID, strategy, parent.ID); err != nil {
		return MergeResult{}, fmt.Errorf("merge: set pending: %w", err)
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
	if err := u.resyncSummary(ctx, parent.ID, parent.WorktreePath, parent.ForkPointSha); err != nil {
		slog.WarnContext(ctx, "merge: parent summary resync failed; read-model will self-correct", "workspace_id", parent.ID, "err", err)
	}
	if err := u.resyncSummary(ctx, child.ID, child.WorktreePath, tip); err != nil {
		slog.WarnContext(ctx, "merge: child summary resync failed; read-model will self-correct", "workspace_id", child.ID, "err", err)
	}
	return MergeResult{ParentTipSha: tip}, nil
}

// resyncSummary recomputes a workspace's working-tree summary from git and
// pushes it into the read model so Added/Deleted/HasCommits/HasConflicts reflect
// the post-merge state of both the parent and the kept child.
func (u *worktreeUsecase) resyncSummary(
	ctx context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) error {
	added, deleted, hasConflicts, hasCommits, err := u.git.WorkingTreeSummary(ctx, worktreePath, forkPointSha)
	if err != nil {
		return fmt.Errorf("merge: resync summary: %w", err)
	}
	_, err = u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           id,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	}, u.now())
	if err != nil {
		return fmt.Errorf("merge: resync summary: sync: %w", err)
	}
	return nil
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

func (u *worktreeUsecase) replayAndReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	tip string,
) (domain.Workspace, error) {
	err := u.git.RebaseOnto(ctx, child.WorktreePath, tip, child.ForkPointSha, child.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: rebase onto: %w", err)
	}
	return u.workspaces.Reparent(ctx, child.ID, newParentID, tip, u.now())
}

func (u *worktreeUsecase) guardReparent(
	ctx context.Context,
	child domain.Workspace,
	newParent domain.Workspace,
) error {
	if newParent.Locked {
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
	order := cascade.Plan(rootID, nodesFrom(all))
	index := indexByID(all)
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
		return err
	}
	if removeErr := u.git.WorktreeRemove(ctx, repoPath, ws.WorktreePath); removeErr != nil {
		return fmt.Errorf("worktree remove: %w", removeErr)
	}
	if delErr := u.git.ForceDeleteBranch(ctx, repoPath, ws.Branch); delErr != nil {
		return fmt.Errorf("branch delete: %w", delErr)
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

func (u *worktreeUsecase) childHasChildren(
	ctx context.Context,
	childID string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, err
	}
	for _, ws := range all {
		if ws.ParentID == childID {
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
			Locked: ws.Locked,
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
