// Package handlers holds the gin handlers backing the chat-keyed worktree
// endpoint: the batch branch import, the seven worktree lifecycle verbs, and
// the branch rename.
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
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
		p workspace.ImportProgress,
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
		consent domain.DeleteConsent,
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

// Worktrees resolves a chat to the workspace behind its worktree (its own, else its nearest
// ancestor's); every verb finds its target through it since no request names a workspace id.
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

// Chats reads a chat by id, so an import can refuse a chat id already taken.
type Chats interface {
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
}

// Operations starts the slow worktree verbs as operations. It is satisfied by
// the operation service.
type Operations interface {
	Start(
		ctx context.Context,
		spec operation.Spec,
		work operation.Work,
	) (domain.Operation, error)
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
// usecase, the worktree hierarchy usecase, the repository store, and the
// operation service the slow verbs run on.
type Handlers struct {
	reader    Reader
	hierarchy Hierarchy
	repos     Repos
	ops       Operations
	chats     Chats
	remote    RemoteRefs
	worktrees Worktrees
}

// New builds the worktree Handlers from the workspace read usecase, the
// worktree hierarchy usecase, the repository store, the operation service and
// the chat reads an import checks its chat ids against.
func New(
	reader Reader,
	hierarchy Hierarchy,
	repos Repos,
	ops Operations,
	chats Chats,
) *Handlers {
	return &Handlers{
		reader:    reader,
		hierarchy: hierarchy,
		repos:     repos,
		ops:       ops,
		chats:     chats,
	}
}

// WithWorktrees wires the chat→workspace resolver every verb runs on. Register does not mount
// the verbs without one, so no route answers for a chat it cannot resolve.
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
