package workspace

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type CreateInput struct {
	ID            string
	RepoID        string
	ProjectID     string
	Branch        string
	WorktreePath  string
	ForkPointSha  string
	ParentID      string
	Protected     bool
	MergeStrategy gitdomain.MergeStrategy
	IsDefault     bool
	Kind          domain.WorkspaceKind
	HeldByPath    string
	CreatedBranch bool
	Provisioning  domain.WorkspaceProvisioning
}

type SyncInput struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

type ProviderInput struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

type Workspace interface {
	Create(
		ctx context.Context,
		in CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	ObserveWorkingTreeState(
		ctx context.Context,
		in SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SyncProviderState(
		ctx context.Context,
		in ProviderInput,
		now time.Time,
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
	TouchActivity(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	Reparent(
		ctx context.Context,
		id string,
		parentID string,
		forkPointSha string,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	UpdateForkPoint(
		ctx context.Context,
		id string,
		forkPointSha string,
	) (domain.Workspace, error)
	ProvisionInPlace(
		ctx context.Context,
		id string,
		worktreePath string,
		forkPointSha string,
	) (domain.Workspace, error)
	ClearBranch(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	RenameBranch(
		ctx context.Context,
		id string,
		branch string,
	) (domain.Workspace, error)
	SetParentFromPR(
		ctx context.Context,
		id string,
		parentID string,
	) (domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
}

type ReconcileOnOpener interface {
	OnOpen(
		ctx context.Context,
		wsID string,
	)
}

type BootSweeper interface {
	BackfillProvisioning(
		ctx context.Context,
	)
	Sweep(
		ctx context.Context,
		onPurged func(ctx context.Context, wsID string),
	) error
}

type DeleteReactorRegistrar interface {
	RegisterDeleteReactor(
		root context.Context,
		forgetDependents func(ctx context.Context, wsID string) error,
		removeWorktree func(ctx context.Context, tomb domain.Workspace) error,
		gate *drain.Gate,
		onGiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error),
	) error
}
