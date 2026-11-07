// Package handlers holds the gin handlers backing the .../repos/:repoId/chats
// endpoints, and their re-mount under the project home group.
package handlers

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// ChatTreeUsecase is the Chats-panel tree: folder CRUD, chat placement and the cascading chat delete,
// which must go through the tree because a chat's children are its threads and go with it.
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
	// CreateChat places the new chat before starting its CLI, so a thread knows its parent from its first
	// session. A fork or import worktree is minted and set on the chat in the same call, never observable
	// apart; in.WorkspaceID is ignored for both.
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
	AnswerMove(
		ctx context.Context,
		chatID, requestID string,
		allow bool,
		reason string,
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

// Repos resolves :repoId so an importing create can name its repo's path and remote outright: an
// import rooted at the repo has no parent workspace to inherit them from.
type Repos interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Repository, error)
}

// Handlers serves the .../repos/:repoId/chats routes from the agent usecase
// and the Chats-panel tree usecase.
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

// New builds the Handlers, one parameter per concern so every capability a route needs is a field the
// compiler checks.
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

// WithRepos wires the store an importing create reads repo facts from; unwired, import is refused
// rather than guessed. A builder, not a New parameter, because the home mount never imports.
func (h *Handlers) WithRepos(
	repos Repos,
) *Handlers {
	if repos != nil {
		h.repos = repos
	}
	return h
}

// WithWorktrees wires the workspace reads behind a chat's git fields. Optional because surfaces mounted
// without it serve no worktree-owning rows; unwired, the worktree is simply absent.
func (h *Handlers) WithWorktrees(
	worktrees Worktrees,
) *Handlers {
	if worktrees != nil {
		h.worktrees = worktrees
	}
	return h
}

// WithNodes wires the Node read a worktree-owning chat's sidebar FolderID/Order come from; unwired,
// they serialize as zero values.
func (h *Handlers) WithNodes(
	nodes Nodes,
) *Handlers {
	if nodes != nil {
		h.nodes = nodes
	}
	return h
}
