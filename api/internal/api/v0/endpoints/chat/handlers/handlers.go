// Package handlers holds the gin handlers backing the .../repos/:repoId/chats
// endpoints, and their re-mount under the project home group.
package handlers

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// ChatTreeUsecase is the Chats-panel tree surface the handlers need: folder
// CRUD, chat placement, and the cascading chat delete.
//
// The DELETE it serves is the one on .../chats/:id, and it goes through
// here rather than straight to ChatUsecase.PurgeChat because a chat delete in
// this panel is not one chat: a chat's children are THREADS of it, so they go
// with it. Only something holding the tree can know which chats those are.
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
	// CreateChat mints a chat, places it under parentID (a chat, a folder, or "" for
	// the panel root) and starts providerID's CLI on it — in that order, so a chat
	// created as a THREAD carries the parent edge before its first CLI exists and is
	// told what it reads on its very first session. runnerID is the crowbarSegmentID
	// every hook from that CLI carries.
	//
	// worktree is model spec §4.1/§5.1's atomic create, in its three real states
	// (agentusecase.WorktreeSpec): a plain chat, a fresh fork off the new chat's
	// resolved fork parent, or an existing branch imported — the last two both
	// minting the workspace and setting it on the chat in the SAME call, so the
	// two effects are never observable apart. workspaceID is ignored for both.
	//
	// surface is the VIEW the chat is born on (design spec 2.5), "" for the
	// provider's own default landing — see domain.Chat.Surface.
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
	DeleteChat(
		ctx context.Context,
		chatID string,
		consent domain.DeleteConsent,
	) (agentusecase.ChatDeletion, error)
}

// Repos resolves the repository named by :repoId, so an IMPORTING create can
// describe the branch it is adopting: a WorktreeImport names its repo outright
// — the on-disk path git works in and the remote the branch is fetched from —
// rather than inheriting them from a parent workspace an import rooted at the
// repo does not have (see agentusecase.ImportSpec).
//
// It is the narrow slice this consumer needs and nothing more (law 4); the
// container satisfies it with the real repository store (law 6).
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

// New builds the agent Handlers from the five agent concerns and the
// Chats-panel tree usecase.
//
// The concerns arrive one per parameter rather than behind one port so that
// every capability a route needs is a field the compiler can check.
//
// A nil tree usecase does NOT degrade: every route that
// takes one would have to answer some fiction about a tree it cannot read, and
// the daemon wires it unconditionally — so the routes are simply not mounted
// without it (see agentusecase.Register).
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

// WithRepos wires the repository store an IMPORTING create reads its repo facts
// from. Left unwired, a create asking to import is refused outright rather than
// guessing them — see Create. Every other route is unaffected, which is why
// this is a builder and not a New parameter: the home mount (home.Register)
// builds these handlers for a surface where import is refused anyway, since it
// always names a workspace to attach to.
func (h *Handlers) WithRepos(
	repos Repos,
) *Handlers {
	if repos != nil {
		h.repos = repos
	}
	return h
}

// WithWorktrees wires the workspace reads a chat's own git fields are resolved
// through (spec §5). Left unwired, every chat serializes with its worktree
// absent — see Worktrees — which is why this is a builder rather than a New
// parameter: it is the same optional-capability shape WithRepos already has,
// and the surfaces that mount these handlers without it serve rows that own no
// worktree anyway.
func (h *Handlers) WithWorktrees(
	worktrees Worktrees,
) *Handlers {
	if worktrees != nil {
		h.worktrees = worktrees
	}
	return h
}

// WithNodes wires the Node read a worktree-owning chat's DTO resolves its own
// sidebar FolderID/Order from (see Nodes, placementReader). Left unwired,
// every worktree-owning chat serializes with FolderID/Order at their zero
// value — the same degrade dto.WorkspacePlacementReader's own doc describes
// for a row this fix has not reached yet — which is why this is a builder
// rather than a New parameter, the same optional-capability shape
// WithWorktrees already has.
func (h *Handlers) WithNodes(
	nodes Nodes,
) *Handlers {
	if nodes != nil {
		h.nodes = nodes
	}
	return h
}
