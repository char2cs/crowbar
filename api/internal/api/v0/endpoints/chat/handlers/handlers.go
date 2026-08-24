// Package handlers holds the gin handlers backing the
// .../workspaces/:wsId/agent endpoints.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	agentchatfolder "github.com/char2cs/crowbar/api/internal/app/usecases/chat/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ChatUsecase is the chat aggregate as the handlers need it: the rows a
// workspace's Chats panel lists, one chat's own record, and the writes that
// retitle it or pin the model its next CLI is launched with.
//
// It starts no processes and reads no vendor CLI. Every route served off here
// answers whether or not a runner has ever been placed on the chat.
type ChatUsecase interface {
	// ListChatsByWorkspace returns every AgentChat anchored to workspaceID
	// (Task 3: List is scoped by the :wsId path param, not global).
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)

	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)

	ReadMessages(
		ctx context.Context,
		chatID string,
		after, before, limit int,
	) (chatlog.Page, error)

	// AssembleHandoff resolves chatID's ledger into the legible handoff blob a
	// freshly spawned provider CLI can be given as prior context.
	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)

	// RenameChat sets chatID's title under user>agent>derived precedence (see
	// agentusecase.ChatUsecase.RenameChat). Broadcasts "titled" on a successful change.
	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error

	// SetChatSelection writes chatID's sticky model and reasoning-effort choice.
	// Empty clears back to the provider's default; a value outside the provider's
	// declared catalogue is refused with apperr.ErrInvalidArgument (→ 400).
	SetChatSelection(
		ctx context.Context,
		chatID, model, effort string,
	) error

	// PurgeChat hard-deletes chatID via asynx Forget, then best-effort kills the
	// vendor CLI that was pointed at it. The chat is fully erased — gone from every
	// read, including a direct GetChat by id — the instant this returns.
	//
	// No route reaches it directly: the panel's DELETE goes through
	// ChatTreeUsecase, because a chat's threads go with it and only the tree knows
	// which those are.
	PurgeChat(
		ctx context.Context,
		chatID string,
	) error
}

// TurnUsecase is the vendor CLI's hook ingress and the record it writes: what
// the agent DID, and what it is blocked on.
type TurnUsecase interface {
	// IngestHook applies one canonical hook event that carries no delivery id —
	// the daemon's own replay, and any forwarder predating the relay journal.
	IngestHook(
		ctx context.Context,
		runnerID string,
		provider string,
		event string,
		rawPayload []byte,
	) error

	// IngestHookDelivery is the exactly-once ingress: the relay mints one delivery
	// id and reuses it on every retry, and this path turns those retries into ONE
	// semantic hook.
	//
	// It is declared here rather than discovered at runtime on purpose. A port
	// that only MIGHT carry it is a port a mis-wire silently falls off — every
	// hook takes the un-journalled path and every retry applies its effects twice
	// — and nothing fails until a user sees the same turn twice in production.
	IngestHookDelivery(
		ctx context.Context,
		workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
		rawPayload []byte,
	) error

	// ReadActivity is what the agent DID: tool calls, subagents, interruptions.
	ReadActivity(
		ctx context.Context,
		chatID string,
		after int64,
		limit int,
	) (agentusecase.ChatActivity, error)

	// ReadToolPayload resolves one tool call's request or result on demand.
	ReadToolPayload(
		ctx context.Context,
		chatID, toolID, side string,
	) ([]byte, error)

	// ReadPendingChoices is what the agent is BLOCKED on: the prompts it has put to
	// a human and not yet had answered. Separate from ReadActivity because it is
	// asked far more often and must not carry a turn's tool calls with it.
	ReadPendingChoices(
		ctx context.Context,
		chatID string,
	) ([]domain.ActivityChoice, error)

	// Telemetry is the provider's own report of cost and capacity, absent until
	// the provider makes one.
	Telemetry(chatID string) (engineagents.Telemetry, bool)
}

// RunnerUsecase is the vendor CLI itself: which one is on a chat, what it has
// been told, and the lifecycle gestures a client can aim at it.
type RunnerUsecase interface {
	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID string,
	) (domain.AgentPromptSubmission, error)

	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagents.SlashCatalog, error)

	// LiveRunnerForChat returns the runner PLACED on chatID right now.
	// runner.ErrNotFound is not a failure here: it means the chat is DORMANT —
	// no live row exists, because no PTY does. Row-existence IS the liveness answer,
	// which is why the handlers ask no second question and no aggregate carries a
	// status flag that could disagree.
	LiveRunnerForChat(
		ctx context.Context,
		chatID string,
	) (engineagents.Runner, error)

	// ConversationsForChat returns every conversation chatID has hosted, oldest
	// first — the append-only history that replaced the chat's embedded segments,
	// and the source of the dormant chat's provider fallback.
	ConversationsForChat(
		ctx context.Context,
		chatID string,
	) ([]engineagents.ChatConversation, error)

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

	// TerminalWait is what the agent is blocked on that Crowbar CANNOT answer: a
	// modal reaching the daemon through no hook, so the only way past it is the
	// terminal. The complement of ReadPendingChoices, and deliberately not folded
	// into it — a choice is a question with buttons, this is a signpost.
	//
	// It takes no ctx and returns no error because it reads a standing answer the
	// detector keeps current on its own cadence; a REST read must not be able to
	// drive provider-screen work on the request path.
	TerminalWait(
		chatID string,
	) domain.AgentTerminalWait
}

// AnswerUsecase is the answer channel: the human deciding a prompt in the chat,
// and the in-PTY relay holding the provider's gate open until they do.
type AnswerUsecase interface {
	// AnswerableChoiceIDs narrows a set of prompts to the ones a relay is holding
	// the provider's gate open for right now. A prompt outside it is still worth
	// drawing — the CLI is asking it — but answering it would reach nobody.
	AnswerableChoiceIDs(
		chatID string,
		choices []domain.ActivityChoice,
	) []string

	// AnswerChoice records a human's decision and hands it to the relay blocking the
	// provider's gate. optionIDs name what was picked; reason and content carry the
	// free-form parts a provider accepts (a deny message, an elicitation's filled-in
	// form). It fails with apperr.ErrConflict when no relay is waiting, because an
	// answer that reaches nobody must never be reported as delivered.
	AnswerChoice(
		ctx context.Context,
		chatID, choiceID string,
		optionIDs []string,
		reason string,
		content []byte,
	) error

	// PendingAnswer reports whether the relay that just delivered deliveryID must
	// stay alive waiting for a human, and how long the daemon will hold it.
	PendingAnswer(deliveryID string) (agentusecase.PendingAnswer, bool)

	// AwaitAnswer blocks that relay until its prompt is decided, the budget expires,
	// or the request is cancelled. Only the first carries bytes to print.
	AwaitAnswer(
		ctx context.Context,
		deliveryID string,
	) (agentusecase.HookAnswer, error)

	// AbandonAnswer is the relay reporting that its prompt was decided somewhere
	// else — it was signalled, which is the only notice a terminal-side DECLINE
	// gives. It clears the prompt so the chat stops showing a question nobody is
	// asking.
	AbandonAnswer(
		ctx context.Context,
		deliveryID string,
	) error
}

// ProviderUsecase is the global provider table and the MCP transport the vendor
// CLIs call back into. Nothing here is workspace-scoped.
type ProviderUsecase interface {
	// ResolveProviders returns the enriched, priority-ordered provider list the
	// backend owns: the descriptor catalog joined with the global preference table
	// and the install probe (connected + enabled, in priority order). It takes no
	// wsId — providers are global — and backs the enriched GET .../chats/providers.
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	// ReplaceProviderPreferences rewrites the whole global preference table from the
	// submitted ordered set (array position → priority), validating ids against the
	// catalog (unknown → apperr.ErrInvalidArgument → 400) and returning the freshly
	// resolved list. It backs PUT /v0/settings/chat/providers.
	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]domain.AgentProvider, error)

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
}

// ChatTreeUsecase is the Chats-panel tree surface the handlers need: folder
// CRUD, chat placement, and the cascading chat delete.
//
// The DELETE it serves is the one on .../chats/:id, and it goes through
// here rather than straight to ChatUsecase.PurgeChat because a chat delete in
// this panel is not one chat: a chat's children are THREADS of it, so they go
// with it. Only something holding the tree can know which chats those are.
type ChatTreeUsecase interface {
	ListInWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.ChatFolder, error)
	Create(
		ctx context.Context,
		in agentchatfolder.CreateInput,
	) (domain.ChatFolder, []domain.ChatFolder, error)
	Rename(
		ctx context.Context,
		workspaceID string,
		id string,
		name string,
	) (domain.ChatFolder, error)
	Move(
		ctx context.Context,
		workspaceID string,
		id string,
		in agentchatfolder.MoveInput,
	) (domain.ChatFolder, []domain.ChatFolder, error)
	Delete(
		ctx context.Context,
		workspaceID string,
		id string,
	) ([]domain.ChatFolder, error)
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
	) (domain.Chat, []domain.ChatFolder, error)
	DeleteChat(
		ctx context.Context,
		chatID string,
	) (agentchatfolder.ChatDeletion, error)
}

// Handlers serves the .../workspaces/:wsId/agent routes from the agent usecase
// and the Chats-panel tree usecase.
type Handlers struct {
	chats           ChatUsecase
	turns           TurnUsecase
	runners         RunnerUsecase
	answers         AnswerUsecase
	providers       ProviderUsecase
	folders         ChatTreeUsecase
	broadcastFolder func(folderID, workspaceID, kind string)
}

// New builds the agent Handlers from the five agent concerns, the Chats-panel
// tree usecase, and the chat-folder broadcast seam.
//
// The concerns arrive one per parameter rather than behind one port so that
// every capability a route needs is a field the compiler can check: a hook
// ingress that is merely PROBABLY there is a hook ingress a mis-wire drops
// silently (see TurnUsecase.IngestHookDelivery).
//
// A nil broadcast degrades to a no-op so the handler never panics when wired
// without a hub (tests). A nil tree usecase does NOT degrade: every route that
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
	broadcastFolder func(folderID, workspaceID, kind string),
) *Handlers {
	if broadcastFolder == nil {
		broadcastFolder = func(_, _, _ string) {}
	}
	return &Handlers{
		chats:           chats,
		turns:           turns,
		runners:         runners,
		answers:         answers,
		providers:       providers,
		folders:         folders,
		broadcastFolder: broadcastFolder,
	}
}
