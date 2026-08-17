// Package handlers holds the gin handlers backing the
// .../workspaces/:wsId/agent endpoints.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agentchatfolder"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// AgentUsecase is the agentic-chat usecase surface the handlers need: spawning a
// chat and the vendor CLI (the RUNNER) that talks to it, ingesting that CLI's
// hooks, and reading chats back.
type AgentUsecase interface {
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

	ReadMessages(
		ctx context.Context,
		chatID string,
		after, before, limit int,
	) (ledger.Page, error)

	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID string,
	) (dto.PromptSubmissionDTO, error)

	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagent.SlashCatalog, error)

	// LiveRunnerForChat returns the runner PLACED on chatID right now.
	// agentrunner.ErrNotFound is not a failure here: it means the chat is DORMANT —
	// no live row exists, because no PTY does. Row-existence IS the liveness answer,
	// which is why the handlers ask no second question and no aggregate carries a
	// status flag that could disagree.
	LiveRunnerForChat(
		ctx context.Context,
		chatID string,
	) (domain.AgentRunner, error)

	// ConversationsForChat returns every conversation chatID has hosted, oldest
	// first — the append-only history that replaced the chat's embedded segments,
	// and the source of the dormant chat's provider fallback.
	ConversationsForChat(
		ctx context.Context,
		chatID string,
	) ([]domain.ChatConversation, error)

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

	// StopChat gracefully terminates chatID's live vendor CLI and leaves the chat
	// DORMANT and resumable (the chat entry and its bound conversation are retained) —
	// the counterpart of ResumeChat, and what closing a chat tab calls. It neither
	// respawns (unlike SwitchProvider) nor deletes the chat (unlike PurgeChat), and the
	// in-flight turn is aborted by design ("close = stop"). A chat with no live runner
	// is a nil no-op.
	StopChat(
		ctx context.Context,
		chatID string,
	) error

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

	// DispatchMCP runs one MCP JSON-RPC message for the runner named by runnerID,
	// authenticated by token. It is the single seam onto the agent tool surface —
	// the handler carries bytes and decides nothing — and the message is passed
	// through raw because the JSON-RPC framing is the engine's business.
	//
	// The bool reports whether a reply should be sent: a JSON-RPC notification is
	// answered with silence.
	DispatchMCP(
		ctx context.Context,
		runnerID string,
		token string,
		message []byte,
	) ([]byte, bool, error)

	// PurgeChat hard-deletes chatID via asynx Forget, then best-effort kills the
	// vendor CLI that was pointed at it. The chat is fully erased — gone from every
	// read, including a direct GetChat by id — the instant this returns.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error

	// ResolveProviders returns the enriched, priority-ordered provider list the
	// backend owns: the descriptor catalog joined with the global preference table
	// and the install probe (connected + enabled, in priority order). It takes no
	// wsId — providers are global — and backs the enriched GET .../agent/providers.
	ResolveProviders(
		ctx context.Context,
	) ([]dto.AgentProviderDTO, error)

	// ReplaceProviderPreferences rewrites the whole global preference table from the
	// submitted ordered set (array position → priority), validating ids against the
	// catalog (unknown → apperr.ErrInvalidArgument → 400) and returning the freshly
	// resolved list. It backs PUT /v0/settings/agent/providers.
	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]dto.AgentProviderDTO, error)
}

// ChatTreeUsecase is the Chats-panel tree surface the handlers need: folder
// CRUD, chat placement, and the cascading chat delete.
//
// The DELETE it serves is the one on .../agent/chats/:id, and it goes through
// here rather than straight to AgentUsecase.PurgeChat because a chat delete in
// this panel is not one chat: a chat's children are THREADS of it, so they go
// with it. Only something holding the tree can know which chats those are.
type ChatTreeUsecase interface {
	ListInWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.AgentChatFolder, error)
	Create(
		ctx context.Context,
		in agentchatfolder.CreateInput,
	) (domain.AgentChatFolder, []domain.AgentChatFolder, error)
	Rename(
		ctx context.Context,
		workspaceID string,
		id string,
		name string,
	) (domain.AgentChatFolder, error)
	Move(
		ctx context.Context,
		workspaceID string,
		id string,
		in agentchatfolder.MoveInput,
	) (domain.AgentChatFolder, []domain.AgentChatFolder, error)
	Delete(
		ctx context.Context,
		workspaceID string,
		id string,
	) ([]domain.AgentChatFolder, error)
	// CreateChat mints a chat, places it under parentID (a chat, a folder, or "" for
	// the panel root) and starts providerID's CLI on it — in that order, so a chat
	// created as a THREAD carries the parent edge before its first CLI exists and is
	// told what it reads on its very first session. runnerID is the crowbarSegmentID
	// every hook from that CLI carries.
	CreateChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
		parentID string,
	) (chatID, runnerID string, err error)
	PlaceChat(
		ctx context.Context,
		workspaceID string,
		chatID string,
		in agentchatfolder.PlaceInput,
	) (domain.AgentChat, []domain.AgentChatFolder, error)
	DeleteChat(
		ctx context.Context,
		chatID string,
	) (agentchatfolder.ChatDeletion, error)
}

// Handlers serves the .../workspaces/:wsId/agent routes from the agent usecase
// and the Chats-panel tree usecase.
type Handlers struct {
	usecase         AgentUsecase
	folders         ChatTreeUsecase
	broadcastFolder func(folderID, workspaceID, kind string)
}

// New builds the agent Handlers from the agent usecase, the Chats-panel tree
// usecase, and the chat-folder broadcast seam.
//
// A nil broadcast degrades to a no-op so the handler never panics when wired
// without a hub (tests). A nil tree usecase does NOT degrade: every route that
// takes one would have to answer some fiction about a tree it cannot read, and
// the daemon wires it unconditionally — so the routes are simply not mounted
// without it (see agent.Register).
func New(
	usecase AgentUsecase,
	folders ChatTreeUsecase,
	broadcastFolder func(folderID, workspaceID, kind string),
) *Handlers {
	if broadcastFolder == nil {
		broadcastFolder = func(_, _, _ string) {}
	}
	return &Handlers{usecase: usecase, folders: folders, broadcastFolder: broadcastFolder}
}
