// Package handlers holds the gin handlers backing the /v0/agent endpoints.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
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

	ListChats(
		ctx context.Context,
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

	// AssembleHandoff resolves chatID's ledger into the legible handoff blob a
	// freshly spawned provider CLI can be given as prior context.
	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)
}

// Handlers serves the /v0/agent routes from the agent usecase.
type Handlers struct {
	usecase AgentUsecase
}

// New builds the agent Handlers from the agent usecase.
func New(
	usecase AgentUsecase,
) *Handlers {
	return &Handlers{usecase: usecase}
}
