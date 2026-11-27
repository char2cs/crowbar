package handlers

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type ChatTreeUsecase interface {
	ListInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)
	ListInHome(
		ctx context.Context,
		homeID string,
	) ([]domain.Chat, error)
	FolderScope(ctx context.Context, id string) (tree.Folder, error)
	Create(
		ctx context.Context,
		in agentusecase.CreateInput,
	) (domain.Chat, []domain.Chat, error)
	Rename(
		ctx context.Context,
		id string,
		name string,
	) (domain.Chat, error)
	Move(
		ctx context.Context,
		id string,
		in agentusecase.MoveInput,
	) (domain.Chat, []domain.Chat, error)
	Delete(
		ctx context.Context,
		id string,
	) ([]domain.Chat, error)
	CreateChat(
		ctx context.Context,
		in agentusecase.CreateChatInput,
	) (agentusecase.CreatedChat, error)
	PlaceChat(
		ctx context.Context,
		workspaceID string,
		chatID string,
		in agentusecase.PlaceInput,
	) (domain.Chat, []domain.Chat, error)
	CancelMove(
		ctx context.Context,
		chatID string,
	) error
	BranchOut(
		ctx context.Context,
		chatID string,
		branch string,
	) error
	DeleteChat(
		ctx context.Context,
		chatID string,
		consent domain.DeleteConsent,
	) (agentusecase.ChatDeletion, error)
}

type Repos interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Repository, error)
}

type Handlers struct {
	chats     ChatUsecase
	turns     TurnUsecase
	runners   RunnerUsecase
	answers   AnswerUsecase
	providers ProviderUsecase
	folders   ChatTreeUsecase
	repos     Repos
	worktrees Worktrees
	nodes     Nodes
}

func New(
	chats ChatUsecase,
	turns TurnUsecase,
	runners RunnerUsecase,
	answers AnswerUsecase,
	providers ProviderUsecase,
	folders ChatTreeUsecase,
) *Handlers {
	return &Handlers{
		chats:     chats,
		turns:     turns,
		runners:   runners,
		answers:   answers,
		providers: providers,
		folders:   folders,
	}
}

func (h *Handlers) WithRepos(
	repos Repos,
) *Handlers {
	if repos != nil {
		h.repos = repos
	}
	return h
}

func (h *Handlers) WithWorktrees(
	worktrees Worktrees,
) *Handlers {
	if worktrees != nil {
		h.worktrees = worktrees
	}
	return h
}

func (h *Handlers) WithNodes(
	nodes Nodes,
) *Handlers {
	if nodes != nil {
		h.nodes = nodes
	}
	return h
}
