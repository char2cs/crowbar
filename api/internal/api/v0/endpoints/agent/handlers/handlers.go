// Package handlers holds the gin handlers backing the
// .../workspaces/:wsId/agent endpoints.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// AgentUsecase is the agentic-chat usecase surface the handlers need:
// spawning a chat's first provider segment, ingesting vendor-CLI hooks, and
// reading back chats/segments.
type AgentUsecase interface {
	SpawnChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
	) (chatID, segID string, err error)

	IngestHook(
		ctx context.Context,
		crowbarSegID string,
		provider string,
		event string,
		rawPayload []byte,
	) error

	// ListChatsByWorkspace returns every AgentChat anchored to workspaceID
	// (Task 3: List is scoped by the :wsId path param, not global).
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.AgentChat, error)

	GetChat(
		ctx context.Context,
		id string,
	) (domain.AgentChat, error)

	SegmentsFor(
		ctx context.Context,
		chatID string,
	) ([]domain.AgentSegment, error)

	// SwitchProvider terminates chatID's active provider CLI, hands off the
	// accumulated context, and spawns targetProviderID as a new segment in the
	// same chat, returning the new segment's id.
	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
	) (newSegID string, err error)

	// ResumeChat revives a chat whose vendor CLI is gone (it exited, or died with
	// the daemon), resuming the last provider into its own native session. A chat
	// that is still live is a no-op: its active segment id comes straight back.
	ResumeChat(
		ctx context.Context,
		chatID string,
	) (segID string, err error)

	// AssembleHandoff resolves chatID's ledger into the legible handoff blob a
	// freshly spawned provider CLI can be given as prior context.
	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)

	// RenameChat sets chatID's title under user>agent>derived precedence (see
	// (*agent.Usecase).RenameChat). Broadcasts "titled" on a successful change.
	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error

	// PurgeChat hard-deletes chatID via asynx Forget after best-effort
	// terminating its active segment's PTY (Task 5). The chat is fully
	// erased — gone from every read, including a direct GetChat by id — the
	// instant this returns.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error

	// ListProviders enumerates the registered agent providers for the workspace
	// (the route ignores which workspace — the descriptor set is global — but the
	// usecase resolves crowbar home from it to read on-disk overrides).
	ListProviders(
		ctx context.Context,
		workspaceID string,
	) ([]engineagent.Descriptor, error)
}

// Handlers serves the .../workspaces/:wsId/agent routes from the agent usecase.
type Handlers struct {
	usecase AgentUsecase
}

// New builds the agent Handlers from the agent usecase.
func New(
	usecase AgentUsecase,
) *Handlers {
	return &Handlers{usecase: usecase}
}
