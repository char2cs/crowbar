// Package handlers holds the gin handlers backing the workspaces endpoint: the
// flat list and detail reads, worktree-backed create and cascade delete, and
// the hierarchy operations (local merge-into-parent and reparent).
package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Reader is the workspace read surface the handlers need: list every workspace
// row from the read model, fetch one by id, sync the working-tree state on
// demand, and resolve a row's merge eligibility against its sibling set.
type Reader interface {
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
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	MergeEligibilityFor(
		ctx context.Context,
		ws domain.Workspace,
		siblings []domain.Workspace,
	) workspace.MergeEligibility
}

// Hierarchy is the worktree-orchestration surface the handlers need: create a
// worktree-backed child, cascade-delete a subtree, run a local child→parent
// merge, and reparent a leaf child onto a new parent (07).
type Hierarchy interface {
	CreateChild(
		ctx context.Context,
		in worktree.CreateChildInput,
	) (domain.Workspace, error)
	CreateFromImport(
		ctx context.Context,
		in worktree.ImportInput,
	) error
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (worktree.MergeResult, error)
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
	// RetryProvision re-provisions a placeholder workspace in place (spec §3.3).
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DetachHolder frees the placeholder's branch from its holder (with consent),
	// then re-provisions in place (spec §3.5/§3.7).
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// RenameBranch renames a managed workspace's branch and relocates its
	// workspace root to match.
	RenameBranch(
		ctx context.Context,
		wsID string,
		newBranch string,
	) (domain.Workspace, error)
}

// Repos resolves a repository by id so the create handler can derive the repo's
// on-disk path and owning project from the request's repoId.
type Repos interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Repository, error)
}

// LastErrorSetter records the message from a failed background mutation on the
// workspace entity so the failure is delivered on the workspace WebSocket stream
// (00 §4: errors live on the entity, never a separate WS frame).
type LastErrorSetter interface {
	SetLastError(
		ctx context.Context,
		id string,
		message string,
	) (domain.Workspace, error)
}

// WorkSignal brackets a workspace's background mutation window so the daemon
// serves a real Working overlay: BeginWork re-broadcasts the row with
// Working=true the moment an async op is accepted, EndWork resolves it, and
// WorkingFor overlays the REST reads so a list/detail fetched mid-work agrees
// with the live stream. WorkingFor combines BOTH derived overlays — the
// inflight background-mutation signal (IsWorking) AND the agent-turn signal —
// so a REST read reflects an in-progress agent turn exactly like the WS frames
// and snapshot readers do; IsWorking is retained for callers that need only the
// inflight half. Blank ids (a create with no entity yet) are no-ops.
type WorkSignal interface {
	BeginWork(
		ctx context.Context,
		wsID string,
	)
	EndWork(
		ctx context.Context,
		wsID string,
	)
	IsWorking(
		wsID string,
	) bool
	// WorkingFor reports whether the workspace is working via EITHER overlay
	// (inflight mutation OR agent chat mid-turn); it is what the REST handlers
	// stamp domain.Workspace.Working from.
	WorkingFor(
		wsID string,
	) bool
}

// Handlers serves the /v0/workspaces routes from the workspace read usecase, the
// worktree hierarchy usecase, the repository store, the workspace error sink
// that surfaces async-mutation failures on the entity, and the work signal
// that drives the entity's Working overlay around async mutations.
type Handlers struct {
	reader     Reader
	hierarchy  Hierarchy
	repos      Repos
	lastErrors LastErrorSetter
	working    WorkSignal
	// async tracks the detached runAsync ops so callers can block on their real
	// completion instead of guessing with a sleep (see runAsync / WaitAsync).
	async sync.WaitGroup
}

// New builds the workspaces Handlers from the workspace read usecase, the
// worktree hierarchy usecase, the repository store, the workspace error sink,
// and the working-overlay signal.
func New(
	reader Reader,
	hierarchy Hierarchy,
	repos Repos,
	lastErrors LastErrorSetter,
	working WorkSignal,
) *Handlers {
	return &Handlers{
		reader:     reader,
		hierarchy:  hierarchy,
		repos:      repos,
		lastErrors: lastErrors,
		working:    working,
	}
}
