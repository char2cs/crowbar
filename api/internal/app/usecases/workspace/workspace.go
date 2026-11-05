package workspace

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// WorkspaceLifecycleRepo is the workspace repository surface the usecase needs.
type WorkspaceLifecycleRepo interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
		protected bool,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in wsrepo.SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
}

// WorkingTreeGitEngine is the git surface used to recompute the summary and to
// predict whether folding a child into its parent would conflict.
type WorkingTreeGitEngine interface {
	WorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		base string,
	) (added, deleted int, hasConflicts, hasCommits bool, err error)
	WouldMergeConflicts(
		ctx context.Context,
		repoPath string,
		pairs []enginegit.MergePair,
	) []enginegit.MergeVerdict
	// RevParse resolves a ref to a commit SHA; summaryBase uses it only to verify
	// a base branch still resolves in the worktree before diffing against it.
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)
}

// ProjectActivityRollup is the best-effort project lastActivity roll-up surface.
type ProjectActivityRollup interface {
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)
}

// Usecase is the workspace feature's one public face: lifecycle/read over the workspace aggregate plus
// the worktree hierarchy (CreateChild through SetChatObserver, delegated to internal/hierarchy).
// SyncWorkingTreeState is the shared wrapper that recomputes and persists the working-tree summary.
type Usecase interface {
	// List returns every workspace row from the read model.
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)

	// ListInRepo returns every workspace row of one project+repo from the same stores as List, cheaper
	// only because it skips other repos' entities.
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)

	// Get returns the workspace with the given id.
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)

	// SetMergeStrategy writes the mergeStrategy field (used by review PATCH).
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)

	// SetLock records the user's own lock decision, which outranks the provider's protected flag; nil
	// hands it back to the provider. Automatic locking still applies; the override lets the user
	// disagree and survive the next poll.
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
	) (domain.Workspace, error)

	// SyncWorkingTreeState recomputes the working-tree summary from git, issues
	// the sync command, and rolls up project lastActivity best-effort.
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)

	// ResolveConflicts clears a sticky pr-conflicts status once the operation in
	// the workspace's own worktree has been resolved (the git usecase calls this
	// on a successful operation continue).
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)

	// MergeEligibilitiesFor resolves whether each workspace can merge into its local parent, reading
	// parent status from the caller-held siblings; git dry-runs are batched per parent checkout.
	MergeEligibilitiesFor(
		ctx context.Context,
		wss []domain.Workspace,
		siblings []domain.Workspace,
	) []MergeEligibility

	// CreateChild creates a worktree-backed (or workspace-only) child.
	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	// CreateFromImport batch-imports branches as managed workspaces, PR-parented
	// up to a protected/default root, creating missing ancestors.
	CreateFromImport(
		ctx context.Context,
		in ImportInput,
		p ImportProgress,
	) error
	// MergeIntoParent runs a local child→parent merge under one of the three
	// strategies.
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (MergeResult, error)
	// Reparent rebases a leaf child onto a new parent's tip.
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	// RenameBranch renames a managed workspace's branch and relocates its
	// workspace root to the directory the new name derives.
	RenameBranch(
		ctx context.Context,
		wsID string,
		newBranch string,
	) (domain.Workspace, error)
	// RebaseOntoParent is the user-initiated "finish the move" action for a
	// moved-but-conflicting child.
	RebaseOntoParent(
		ctx context.Context,
		childID string,
	) (domain.Workspace, error)
	// RetryProvision re-runs holder resolution + provisioning for an existing
	// placeholder.
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DetachHolder frees a live holder off the placeholder's branch, then
	// Retry-provisions in place.
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DeleteCascade removes rootID and its unlocked descendants.
	// Without consent it refuses over work that exists nowhere else.
	DeleteCascade(
		ctx context.Context,
		rootID string,
		consent domain.DeleteConsent,
	) error
	// DeletionPlan is what DeleteCascade(rootID) would remove, or its refusal.
	DeletionPlan(ctx context.Context, rootID string) ([]string, error)
	// WorkAtRisk is what DeleteCascade of each root would refuse over.
	WorkAtRisk(
		ctx context.Context,
		rootIDs []string,
	) ([]domain.WorkAtRisk, error)
	// DeleteRepoWorkspaces removes every workspace of a repo, taking the repo from the caller since its
	// row may already be gone. It never deletes branches or checkouts Crowbar did not create, never
	// --forces a locked worktree, and without consent refuses over work at risk.
	DeleteRepoWorkspaces(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) error
	// RepoWorkAtRisk is what DeleteRepoWorkspaces would refuse over.
	RepoWorkAtRisk(
		ctx context.Context,
		repo domain.Repository,
	) ([]domain.WorkAtRisk, error)
	// SetChatObserver wires the chat surface guardReparent's working-chat check needs; a setter because
	// the chat usecase itself depends on this one (Promote forks through it).
	SetChatObserver(observer ChatWorkObserver)
	// SetOwningChats wires the chat-side surface every workspace this usecase
	// creates is minted under — see hierarchy.Usecase.SetOwningChats.
	SetOwningChats(chats OwningChats)
}

type workspaceUsecase struct {
	repo   WorkspaceLifecycleRepo
	git    WorkingTreeGitEngine
	rollup ProjectActivityRollup
	hierarchy.Usecase
}

// New builds a Usecase. hierarchyWorkspaces and hierarchyGit are the same repo and git engine as
// repo/git, widened for the hierarchy, kept separate so lifecycle-only callers keep narrow fakes;
// repos resolves a repository's main path so removals never run against a child's own worktree.
func New(
	repo WorkspaceLifecycleRepo,
	git WorkingTreeGitEngine,
	rollup ProjectActivityRollup,
	hierarchyWorkspaces wsrepo.Workspace,
	hierarchyGit enginegit.Engine,
	provider engineprovider.Engine,
	repos store.Store[domain.Repository, string],
	now func() time.Time,
	crowbarHome func() (string, error),
	opts ...Option,
) Usecase {
	return &workspaceUsecase{
		repo:    repo,
		git:     git,
		rollup:  rollup,
		Usecase: hierarchy.New(hierarchyWorkspaces, hierarchyGit, provider, repos, now, crowbarHome, opts...),
	}
}

// List returns every workspace row from the read model.
func (u *workspaceUsecase) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	list, err := u.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return list, nil
}

// ListInRepo returns every workspace row of one project+repo from the same stores as List, cheaper
// only because it skips other repos' entities.
func (u *workspaceUsecase) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	list, err := u.repo.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("workspace: list in repo: %w", err)
	}
	return list, nil
}

// Get returns the workspace with the given id.
func (u *workspaceUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	return ws, nil
}

// SetMergeStrategy writes the mergeStrategy field (used by review PATCH).
func (u *workspaceUsecase) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	ws, err := u.repo.SetMergeStrategy(ctx, id, strategy)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set merge strategy: %w", err)
	}
	return ws, nil
}

// SetLock records the user's own lock decision for a workspace. The provider's protected answer is
// read off the stored row: it only matters when clearing, and a locked row without override is
// locked because the provider said so.
func (u *workspaceUsecase) SetLock(
	ctx context.Context,
	id string,
	locked *bool,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set lock: get: %w", err)
	}
	protected := ws.LockOverride == nil && ws.Status == domain.WorkspaceStatusLocked
	updated, err := u.repo.SetLock(ctx, id, locked, protected)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set lock: %w", err)
	}
	return updated, nil
}

// SyncWorkingTreeState recomputes the working-tree summary from git, issues the
// sync command, and rolls up project lastActivity best-effort.
func (u *workspaceUsecase) SyncWorkingTreeState(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync working tree: get: %w", err)
	}
	in, err := u.summarize(ctx, ws)
	if err != nil {
		return domain.Workspace{}, err
	}
	synced, err := u.repo.SyncWorkingTreeState(ctx, in, now)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync working tree: %w", err)
	}
	u.rollup.TouchProjectActivity(ctx, ws.RepoID, now)
	return synced, nil
}

// ResolveConflicts clears the sticky pr-conflicts status after the operation in the workspace's own
// worktree resolves (called on a successful continue).
func (u *workspaceUsecase) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	ws, err := u.repo.ResolveConflicts(ctx, id, now)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: resolve conflicts: %w", err)
	}
	u.rollup.TouchProjectActivity(ctx, ws.RepoID, now)
	return ws, nil
}

// MergeEligibilitiesFor resolves every workspace's eligibility over one sibling
// set, batching the conflict dry-runs.
func (u *workspaceUsecase) MergeEligibilitiesFor(
	ctx context.Context,
	wss []domain.Workspace,
	siblings []domain.Workspace,
) []MergeEligibility {
	return ResolveMergeEligibilities(ctx, wss, siblings, u.git)
}

func (u *workspaceUsecase) summarize(
	ctx context.Context,
	ws domain.Workspace,
) (wsrepo.SyncInput, error) {
	// A project-level home workspace is rooted at the project directory, which is not a git repo; git
	// would fail every post-write resync. It has no branch or index, so its summary is zero by definition.
	if ws.Kind == domain.WorkspaceKindHome {
		return wsrepo.SyncInput{ID: ws.ID}, nil
	}
	added, deleted, hasConflicts, hasCommits, err := u.git.WorkingTreeSummary(
		ctx,
		ws.WorktreePath,
		u.summaryBase(ctx, ws),
	)
	if err != nil {
		return wsrepo.SyncInput{}, fmt.Errorf("workspace: sync working tree: summary: %w", err)
	}
	return wsrepo.SyncInput{
		ID:           ws.ID,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	}, nil
}

// summaryBase returns the base branch name (parent's for a child, own for a protected root) so the
// summary tracks the base's current tip instead of a stale fork point. It falls back to ForkPointSha
// when the branch is unusable, matching the branch-review pane's fallback.
func (u *workspaceUsecase) summaryBase(
	ctx context.Context,
	ws domain.Workspace,
) string {
	base := ws.Branch
	if ws.ParentID != "" {
		parent, err := u.repo.Get(ctx, ws.ParentID)
		if err != nil {
			return ws.ForkPointSha
		}
		base = parent.Branch
	}
	if base == "" {
		return ws.ForkPointSha
	}
	if _, err := u.git.RevParse(ctx, ws.WorktreePath, base); err != nil {
		return ws.ForkPointSha
	}
	return base
}
