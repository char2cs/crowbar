package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type Reader interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
	) (domain.Workspace, error)
}

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
	GuardMergeFold(ctx context.Context, childID string) error
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
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	RenameBranch(
		ctx context.Context,
		wsID string,
		newBranch string,
	) (domain.Workspace, error)
}

type Worktrees interface {
	Resolve(
		ctx context.Context,
		chatID string,
	) (domain.Workspace, error)
}

type Repos interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Repository, error)
}

type Chats interface {
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
}

type Operations interface {
	Start(
		ctx context.Context,
		spec operation.Spec,
		work operation.Work,
	) (domain.Operation, error)
}

type RemoteRefs interface {
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
	reader    Reader
	hierarchy Hierarchy
	repos     Repos
	ops       Operations
	chats     Chats
	remote    RemoteRefs
	worktrees Worktrees
}

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

func (h *Handlers) WithWorktrees(
	worktrees Worktrees,
) *Handlers {
	if worktrees != nil {
		h.worktrees = worktrees
	}
	return h
}

func (h *Handlers) WithRemoteRefs(
	remote RemoteRefs,
) *Handlers {
	if remote != nil {
		h.remote = remote
	}
	return h
}
