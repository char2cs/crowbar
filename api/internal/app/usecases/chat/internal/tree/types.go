package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type Chats interface {
	ListByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	LoadChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	SetPlacement(
		ctx context.Context,
		chatID string,
		parentID string,
		order int,
	) (domain.Chat, error)
	SetOrder(
		ctx context.Context,
		chatID string,
		order int,
	) (domain.Chat, error)
}

type Agent interface {
	MintChat(
		ctx context.Context,
		chatID string,
		workspaceID string,
		providerID string,
		surface string,
	) (string, error)
	DefaultProviderID(ctx context.Context) (string, error)
	OpenChat(
		ctx context.Context,
		chatID string,
		providerID string,
	) (string, error)
	SpawnChatWithOwnWorktree(
		ctx context.Context,
		chatID string,
		providerID string,
		branch string,
	) (workspaceID, runnerID string, err error)
	SpawnChatWithImportedWorktree(
		ctx context.Context,
		chatID string,
		providerID string,
		spec ImportSpec,
	) (ws domain.Workspace, runnerID string, err error)
	AttachWorkspace(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) error
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error
	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error
	RequestMove(ctx context.Context, chatID string, m ledger.Move) (bool, error)
	CancelMove(ctx context.Context, chatID, reason string) error
	ExecuteMove(
		ctx context.Context, chatID string, apply func(context.Context, ledger.Move) (ledger.Move, error),
	) (done ledger.Move, ran bool, err error)
	RunIn(ctx context.Context, chatID, workspaceID string) error
	MoveSite(ctx context.Context, workspaceID string) (dir, projectID, repoID string, err error)
	MovesWaiting(ctx context.Context) ([]string, error)
	RequestBranchOut(ctx context.Context, chatID, workspaceID, branch, by string) (bool, error)
	CheckBranchOutName(ctx context.Context, workspaceID, branch string) error
	BranchOutWorkspace(ctx context.Context, workspaceID, branch string) (domain.Workspace, error)
	DiscardWorkspace(ctx context.Context, workspaceID string) error
}

type WorkspaceReaper interface {
	DeletionPlan(ctx context.Context, workspaceID string) ([]string, error)
	DeleteWorkspace(
		ctx context.Context,
		workspaceID string,
		consent domain.DeleteConsent,
	) error
	WorkAtRisk(
		ctx context.Context,
		workspaceIDs []string,
	) ([]domain.WorkAtRisk, error)
}

type CreateInput struct {
	ID       string
	RepoID   string
	ParentID string
	Name     string
	HomeID   string
}

type MoveInput struct {
	ParentID *string
	Order    *int
}

type PlaceInput struct {
	ParentID    *string
	Order       *int
	WorkspaceID string
	Label       string
}

type ChatDeletion struct {
	Chats   []string
	Folders []string
	Shifted []domain.Chat
	Taken   []string
}

type Usecase interface {
	MoveWhenReady(ctx context.Context, chatID string)
	ResumeMoves(ctx context.Context) error
	StopMoves()
	CancelMove(ctx context.Context, chatID string) error
	BranchOut(ctx context.Context, chatID, branch string) error
	ListInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)
	ListInHome(
		ctx context.Context,
		homeID string,
	) ([]domain.Chat, error)
	FolderScope(
		ctx context.Context,
		id string,
	) (dtree.Folder, error)
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.Chat, []domain.Chat, error)
	Rename(
		ctx context.Context,
		id string,
		name string,
	) (domain.Chat, error)
	Move(
		ctx context.Context,
		id string,
		in MoveInput,
	) (domain.Chat, []domain.Chat, error)
	Delete(
		ctx context.Context,
		id string,
	) ([]domain.Chat, error)
	CreateChat(
		ctx context.Context,
		in CreateChatInput,
	) (CreatedChat, error)
	ImportBranchAsChat(
		ctx context.Context,
		chatID string,
		spec ImportSpec,
	) (mintedChatID, workspaceID string, err error)
	MintOwningChat(
		ctx context.Context,
		chatID string,
		parentWorkspaceID string,
	) (string, error)
	AttachOwningWorkspace(
		ctx context.Context,
		chatID string,
		ws domain.Workspace,
	) error
	DiscardOwningChat(
		ctx context.Context,
		chatID string,
	) error
	PlaceChat(
		ctx context.Context,
		workspaceID string,
		chatID string,
		in PlaceInput,
	) (domain.Chat, []domain.Chat, error)
	DeleteChat(
		ctx context.Context,
		chatID string,
		consent domain.DeleteConsent,
	) (ChatDeletion, error)
	PlaceWorkspace(
		ctx context.Context,
		workspaceID string,
		in PlaceInput,
	) (domain.Chat, []domain.Chat, error)
}
