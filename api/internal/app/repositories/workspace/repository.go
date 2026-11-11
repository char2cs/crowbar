package workspace

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// CreateInput carries the fields needed to create a workspace.
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
	// CreatedBranch: Crowbar created Branch for this workspace, so a teardown
	// may delete it (domain.Workspace.CreatedBranch).
	CreatedBranch bool
	// Provisioning is required: what stands behind WorktreePath.
	Provisioning domain.WorkspaceProvisioning
}

// SyncInput carries a recomputed working-tree summary.
type SyncInput struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

// ProviderInput carries a provider poll result.
type ProviderInput struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

// Workspace is the workspace aggregate repository.
type Workspace interface {
	Create(
		ctx context.Context,
		in CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	// SyncWorkingTreeState returns once the synced row is published, so a
	// write an operation makes reaches clients before the operation settles.
	SyncWorkingTreeState(
		ctx context.Context,
		in SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	// ObserveWorkingTreeState is a watcher's sync: nothing waits on its publish.
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
	// SetLock records the user's lock decision, which outranks the provider's
	// protected flag. A nil `locked` hands it back to the provider, whose current
	// answer `protected` then resolves the status.
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
	// ProvisionInPlace attaches a worktree to a placeholder row: it
	// records worktreePath + forkPointSha and clears HeldByPath, keeping Status.
	ProvisionInPlace(
		ctx context.Context,
		id string,
		worktreePath string,
		forkPointSha string,
	) (domain.Workspace, error)
	// ClearBranch blanks an existing aggregate's Branch to "",
	// leaving every other field untouched. Used by the Detach-holder op when the
	// holder is the repo home.
	ClearBranch(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	// RenameBranch records a branch rename. The workspace does not move: its
	// directory is fixed at creation and never tracks the branch, so there is no
	// path to carry. Identity and lineage are untouched.
	RenameBranch(
		ctx context.Context,
		id string,
		branch string,
	) (domain.Workspace, error)
	// SetParentFromPR sets ParentID from an open PR's target branch without
	// recomputing ForkPointSha.
	SetParentFromPR(
		ctx context.Context,
		id string,
		parentID string,
	) (domain.Workspace, error)
	// Delete tombstones the workspace (Status=deleted); the physical teardown runs
	// off the write path in the async delete reactor.
	Delete(
		ctx context.Context,
		id string,
	) error
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	// ListInRepo returns every workspace row in one project+repo from the same read
	// model as List, with the same consistency, so a broadcast resolves siblings
	// without materializing the whole install.
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	// GetHomeForProject returns the home workspace for the given project.
	// Returns problem.ErrNotFound if no home workspace exists yet.
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
}

// ReconcileOnOpener triggers a deduplicated background reconcile when one workspace
// is read by id (List never does). It is injected so this repository keeps no
// git/provider dependency.
type ReconcileOnOpener interface {
	OnOpen(
		ctx context.Context,
		wsID string,
	)
}

// BootSweeper is the boot orphan-sweep seam: run once at boot, it re-drives the
// delete reactor's Purger for every tombstone a crash left. It is kept off
// Workspace so boot recovery does not leak into the per-request surface.
type BootSweeper interface {
	// BackfillProvisioning gives every row written before
	// domain.Workspace.Provisioning existed its explicit value. It runs before
	// anything reads the field, and never fails boot.
	BackfillProvisioning(
		ctx context.Context,
	)
	Sweep(
		ctx context.Context,
		onPurged func(ctx context.Context, wsID string),
	) error
}

// DeleteReactorRegistrar wires the physical purge onto axWorkspace from injected
// cascade, remover, drain gate and give-up hook (tomb nil if never observed); one
// Purger serves both the async delete reactor and the boot Sweep.
type DeleteReactorRegistrar interface {
	RegisterDeleteReactor(
		root context.Context,
		forgetDependents func(ctx context.Context, wsID string) error,
		removeWorktree func(ctx context.Context, tomb domain.Workspace) error,
		gate *drain.Gate,
		onGiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error),
	) error
}
