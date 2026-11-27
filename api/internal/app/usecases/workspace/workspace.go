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
	RevParse(
		ctx context.Context,
		repoPath string,
		rev string,
	) (string, error)
}

type ProjectActivityRollup interface {
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)
}

type Usecase interface {
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
	) (domain.Workspace, error)

	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)

	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)

	MergeEligibilitiesFor(
		ctx context.Context,
		wss []domain.Workspace,
		siblings []domain.Workspace,
	) []MergeEligibility

	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	CheckNewBranch(ctx context.Context, parentID, branch string) error
	CreateFromImport(
		ctx context.Context,
		in ImportInput,
		p ImportProgress,
	) error
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (MergeResult, error)
	GuardMergeFold(ctx context.Context, childID string) error
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	RenameBranch(
		ctx context.Context,
		wsID string,
		newBranch string,
	) (domain.Workspace, error)
	RebaseOntoParent(
		ctx context.Context,
		childID string,
	) (domain.Workspace, error)
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	DeleteCascade(
		ctx context.Context,
		rootID string,
		consent domain.DeleteConsent,
	) error
	DeletionPlan(ctx context.Context, rootID string) ([]string, error)
	WorkAtRisk(
		ctx context.Context,
		rootIDs []string,
	) ([]domain.WorkAtRisk, error)
	DeleteRepoWorkspaces(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) error
	RepoWorkAtRisk(
		ctx context.Context,
		repo domain.Repository,
	) ([]domain.WorkAtRisk, error)
	SetChatObserver(observer ChatWorkObserver)
	SetOwningChats(chats OwningChats)
}

type workspaceUsecase struct {
	repo   WorkspaceLifecycleRepo
	git    WorkingTreeGitEngine
	rollup ProjectActivityRollup
	hierarchy.Usecase
}

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

func (u *workspaceUsecase) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	list, err := u.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return list, nil
}

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
