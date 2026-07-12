// Package handlers holds the gin handlers backing the
// .../workspaces/:wsId/agent endpoints.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// AgentUsecase is the agentic-chat usecase surface the handlers need: spawning a
// chat and the vendor CLI (the RUNNER) that talks to it, ingesting that CLI's
// hooks, and reading chats back.
type AgentUsecase interface {
	// SpawnChat mints a chat and starts a runner on it. runnerID is the
	// crowbarSegmentID every hook from that CLI carries — stable for the life of the
	// process, including across every conversation it moves between.
	SpawnChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
	) (chatID, runnerID string, err error)

	IngestHook(
		ctx context.Context,
		runnerID string,
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

	// SwitchProvider quits the chat's current vendor CLI, hands off the accumulated
	// context, and starts targetProviderID as a new runner on the SAME chat,
	// returning the new runner's id.
	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
	) (newRunnerID string, err error)

	// ResumeChat revives a dormant chat — one no runner points at, because its CLI
	// exited or died with the daemon — resuming the provider that was last here into
	// the conversation it left. A chat that is still live is a no-op: the id of the
	// runner already on it comes straight back.
	ResumeChat(
		ctx context.Context,
		chatID string,
	) (runnerID string, err error)

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

	// RenameByRunner resolves runnerID to the chat it is placed on RIGHT NOW and
	// applies the same user>agent>derived precedence RenameChat does (see
	// (*agent.Usecase).RenameByRunner). It is what the `crowbar chat rename
	// --segment <segid>` CLI calls: the chat id is never baked into the agent's
	// spawn-time instruction, so a CLI that has since moved to a different chat
	// (a /clear or /resume issued inside it) can never rename the chat it used
	// to be on.
	RenameByRunner(
		ctx context.Context,
		runnerID, title, source string,
	) error

	// PurgeChat hard-deletes chatID via asynx Forget, then best-effort kills the
	// vendor CLI that was pointed at it. The chat is fully erased — gone from every
	// read, including a direct GetChat by id — the instant this returns.
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
