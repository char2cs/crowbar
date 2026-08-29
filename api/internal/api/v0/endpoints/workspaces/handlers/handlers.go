// Package handlers holds the gin handlers backing the workspaces endpoint: the
// flat list and detail reads, worktree-backed create and cascade delete, and
// the hierarchy operations (local merge-into-parent and reparent).
package handlers

import (
	"context"
	"sync"
	"time"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Placer is the unified sidebar-tree surface the handlers need: a
// workspace-owning row's folder placement is just that row's Chat.ParentID,
// moved through the same command Task 4's chat/folder rows already use — see
// ChatResolver for how the handlers find the chat row a workspace id owns.
type Placer interface {
	PlaceChat(
		ctx context.Context,
		workspaceID string,
		chatID string,
		in agentusecase.PlaceInput,
	) (domain.Chat, []domain.Chat, error)
}

// ChatResolver resolves the chat row a workspace id owns, so a workspace's own
// folder-placement request can be addressed to that row: every workspace-owning
// row is a chat row (Stage 1's taxonomy), and Placer.PlaceChat is addressed by
// chat id, never by workspace id.
type ChatResolver interface {
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
}

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
	// SetLock records the user's own lock decision; nil hands the question back
	// to the provider's protected flag. See lock.go.
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
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

type Handlers struct {
	reader     Reader
	hierarchy  Hierarchy
	repos      Repos
	lastErrors LastErrorSetter
	working    WorkSignal
	remote     RemoteRefs
	placer     Placer
	chats      ChatResolver
	// broadcastFolder delivers the folder-typed chat rows a placement renumbered
	// — folderID, the workspace scope, and the frame kind — mirroring the Chats
	// panel's own AgentChatFolder broadcast, since a workspace-owning row's
	// placement now writes the very same chat row those folders share a sibling
	// space with.
	broadcastFolder func(folderID string, workspaceID string, kind string)
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
		reader:          reader,
		hierarchy:       hierarchy,
		repos:           repos,
		lastErrors:      lastErrors,
		working:         working,
		broadcastFolder: func(string, string, string) {},
	}
}

// WithPlacer wires the sidebar-placement usecase, the chat-row resolver, and
// the folder broadcast the PATCH and Create handlers need. A nil placer or
// resolver leaves placement unavailable (the handler answers 500), matching
// WithRemoteRefs' degrade-rather-than-panic wiring; a nil broadcast degrades to
// a no-op.
func (h *Handlers) WithPlacer(
	placer Placer,
	chats ChatResolver,
	broadcast func(folderID string, workspaceID string, kind string),
) *Handlers {
	if placer != nil {
		h.placer = placer
	}
	if chats != nil {
		h.chats = chats
	}
	if broadcast != nil {
		h.broadcastFolder = broadcast
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
