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

// ProviderInput carries a provider poll result (08 §5).
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
	SyncWorkingTreeState(
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
	// SetLock records the user's own lock decision, which outranks the
	// provider's protected flag from here on. A nil `locked` hands the question
	// back to the provider. `protected` is the provider's current answer, used
	// only to resolve the status when `locked` is nil.
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
	// ProvisionInPlace attaches a worktree to a placeholder row (spec §3.3): it
	// records worktreePath + forkPointSha and clears HeldByPath, keeping Status.
	ProvisionInPlace(
		ctx context.Context,
		id string,
		worktreePath string,
		forkPointSha string,
	) (domain.Workspace, error)
	// ClearBranch blanks an existing aggregate's Branch to "" (spec §4/B6),
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
	// SetLastError records the message from a failed background operation on the
	// workspace; the failure surfaces on the entity, never a separate WS frame
	// (00 §4). The next successful mutating command clears it.
	SetLastError(
		ctx context.Context,
		id string,
		message string,
	) (domain.Workspace, error)
	// Delete tombstones the workspace: it fires the pure Delete command (folds
	// Status=deleted) via Send. The physical teardown (cascade Forgets + rm -rf +
	// axWorkspace.Forget) runs off the write path in the async delete reactor
	// (spec §3.6/§3.8, Task 8).
	Delete(
		ctx context.Context,
		id string,
	) error
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	// ListInRepo returns every workspace row scoped to one project+repo. It reads
	// the same central durable read model as List (state/store/workspace.db) and
	// filters by projectID+repoID, so it has identical read-after-write
	// consistency to List — the merge-eligibility broadcast overlay uses it so a
	// broadcast of a parented workspace resolves its siblings without materializing
	// the whole install.
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

// ReconcileOnOpener triggers a lazy, deduplicated, one-shot background reconcile
// for a single workspace id off the per-id read path (spec §3.8). Get/detail
// calls OnOpen; List never does. It is injected via WithReconciler so the
// git+provider re-derivation stays owned by a higher layer and this repository
// keeps no git/provider dependency.
type ReconcileOnOpener interface {
	OnOpen(
		ctx context.Context,
		wsID string,
	)
}

// BootSweeper is the boot orphan-sweep seam (spec §3.8). The app-layer
// composition root runs it once at boot; it re-drives the SAME Purger the delete
// reactor runs for every tombstone a crash left behind (spec §7-D). Kept OFF the
// main Workspace interface so a boot-recovery concern does not leak into the
// per-request repository surface; the concrete *workspace satisfies it.
type BootSweeper interface {
	// BackfillProvisioning gives every row written before
	// domain.Workspace.Provisioning existed its explicit value. It runs before
	// anything reads the field, and never fails boot.
	BackfillProvisioning(
		ctx context.Context,
	)
	Sweep(
		ctx context.Context,
	) error
}

// DeleteReactorRegistrar wires this aggregate's physical purge (Task 8) onto its
// singleton axWorkspace. The composition root injects the cross-aggregate forget
// cascade, the hardened worktree remover, the shared drain gate and who hears a
// purge that gives up (tomb nil when it was never observed); this
// repository builds the one Purger from them and hands it to both the async
// delete reactor and the boot Sweep. Kept OFF the main Workspace interface (like
// BootSweeper); the concrete *workspace satisfies it.
type DeleteReactorRegistrar interface {
	RegisterDeleteReactor(
		root context.Context,
		forgetDependents func(ctx context.Context, wsID string) error,
		removeWorktree func(ctx context.Context, tomb domain.Workspace) error,
		gate *drain.Gate,
		onGiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error),
	) error
}
