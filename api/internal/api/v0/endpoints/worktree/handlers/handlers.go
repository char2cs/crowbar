// Package handlers holds the gin handlers backing the chat-keyed worktree
// endpoint: the batch branch import, the seven worktree lifecycle verbs, and
// the branch rename.
package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Reader is the workspace read surface the handlers need: list every workspace
// row (the post-merge leaf check reads the whole set), sync the working-tree
// state on demand, and record the user's own lock decision.
type Reader interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	// SetLock records the user's own lock decision; nil hands the question back
	// to the provider's protected flag. See lock.go.
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
	) (domain.Workspace, error)
}

// Hierarchy is the worktree-orchestration surface the handlers need: adopt a
// batch of existing branches, cascade-delete a subtree, run a local child→parent
// merge, and reparent a leaf child onto a new parent (07).
type Hierarchy interface {
	CreateFromImport(
		ctx context.Context,
		in workspace.ImportInput,
	) error
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (workspace.MergeResult, error)
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

// Worktrees resolves a CHAT to the workspace behind the worktree it reads and
// writes through — itself if it owns one, else its nearest ancestor that does
// (spec docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md §3).
//
// It is how every verb here finds its target, past law 1's refusal to let a
// request name a :wsId, and it is declared HERE, as the narrow slice this
// consumer actually needs (law 4); the container's own resolver satisfies it
// (law 6).
type Worktrees interface {
	Resolve(
		ctx context.Context,
		chatID string,
	) (domain.Workspace, error)
}

// Repos resolves a repository by id so the batch import can derive the repo's
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
// WorkingFor overlays the REST reads so a read fetched mid-work agrees with the
// live stream. WorkingFor combines BOTH derived overlays — the inflight
// background-mutation signal (IsWorking) AND the agent-turn signal; IsWorking is
// retained for callers that need only the inflight half. Blank ids (an import
// with no entity yet) are no-ops.
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
	// (inflight mutation OR agent chat mid-turn).
	WorkingFor(
		wsID string,
	) bool
}

// RemoteRefs is the narrow git surface Import uses to refuse a branch that is
// not on the remote synchronously, on the request path, instead of letting the
// batch fail in the background where it has no entity to report through.
type RemoteRefs interface {
	// FetchPrune refreshes refs/remotes/origin/* so the check below is made
	// against the remote as it is now, not as the clone last heard it.
	FetchPrune(
		ctx context.Context,
		repoPath string,
	) error
	RemoteTrackingBranchExists(
		ctx context.Context,
		repoPath string,
		branch string,
	) (bool, error)
}

// Handlers serves the chat-keyed worktree routes from the workspace read
// usecase, the worktree hierarchy usecase, the repository store, the workspace
// error sink that surfaces async-mutation failures on the entity, and the work
// signal that drives the entity's Working overlay around async mutations.
type Handlers struct {
	reader     Reader
	hierarchy  Hierarchy
	repos      Repos
	lastErrors LastErrorSetter
	working    WorkSignal
	remote     RemoteRefs
	worktrees  Worktrees
	// async tracks the detached runAsync ops so callers can block on their real
	// completion instead of guessing with a sleep (see runAsync / WaitAsync).
	async sync.WaitGroup
}

// New builds the worktree Handlers from the workspace read usecase, the
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

// WithWorktrees wires the chat→workspace resolver every verb runs on. A nil
// resolver does NOT leave a half-working surface: Register simply does not mount
// those routes without one, so the handler never has to answer a fiction about a
// chat it cannot resolve.
func (h *Handlers) WithWorktrees(
	worktrees Worktrees,
) *Handlers {
	if worktrees != nil {
		h.worktrees = worktrees
	}
	return h
}

// WithRemoteRefs wires the git surface Import validates branches against. A nil
// arg leaves that validation off, which degrades to the pre-existing behaviour:
// the batch runs and reports per-branch outcomes as placeholder rows.
func (h *Handlers) WithRemoteRefs(
	remote RemoteRefs,
) *Handlers {
	if remote != nil {
		h.remote = remote
	}
	return h
}
