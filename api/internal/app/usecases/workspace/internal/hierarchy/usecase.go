package hierarchy

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

type Usecase interface {
	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	CheckNewBranch(ctx context.Context, parentID, branch string) error
	CreateFromImport(ctx context.Context, in ImportInput, p ImportProgress) error
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

type TerminalReaper interface {
	ListSessionsForChat(chatID string) []string
	Kill(ctx context.Context, sessionID string) error
}

type ChatWorkObserver interface {
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
	ChatBusy(ctx context.Context, chatID string) (bool, error)
}

type hierarchyUsecase struct {
	workspaces   workspace.Workspace
	git          enginegit.Engine
	provider     engineprovider.Engine
	repos        store.Store[domain.Repository, string]
	now          func() time.Time
	crowbarHome  func() (string, error)
	terminals    TerminalReaper
	chatObserver ChatWorkObserver
	owningChats  OwningChats
	nodes        NodeCreator
}

type Option func(*hierarchyUsecase)

func WithTerminalReaper(r TerminalReaper) Option {
	return func(u *hierarchyUsecase) { u.terminals = r }
}

func New(
	workspaces workspace.Workspace,
	git enginegit.Engine,
	provider engineprovider.Engine,
	repos store.Store[domain.Repository, string],
	now func() time.Time,
	crowbarHome func() (string, error),
	opts ...Option,
) Usecase {
	u := &hierarchyUsecase{
		workspaces:  workspaces,
		git:         git,
		provider:    provider,
		repos:       repos,
		now:         now,
		crowbarHome: crowbarHome,
		nodes:       noOpNodeCreator{},
	}
	for _, o := range opts {
		o(u)
	}
	return u
}

func (u *hierarchyUsecase) SetChatObserver(observer ChatWorkObserver) {
	u.chatObserver = observer
}

func (u *hierarchyUsecase) SetOwningChats(chats OwningChats) {
	u.owningChats = chats
}
