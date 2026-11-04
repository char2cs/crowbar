package handlers

import (
	"context"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// ChatUsecase is the chat aggregate as the handlers need it: the rows a
// workspace's Chats panel lists, one chat's own record, and the writes that
// retitle it or pin the model its next CLI is launched with.
//
// It starts no processes and reads no vendor CLI. Every route served off here
// answers whether or not a runner has ever been placed on the chat.
type ChatUsecase interface {
	// ChatSnapshot is chatID's versioned snapshot: the chat, the runner live on
	// it, its phase and the version that orders it against every frame. Every
	// chat body this API serves is built from it.
	ChatSnapshot(
		ctx context.Context,
		chatID string,
	) (agentusecase.ChatSnapshot, error)

	// ChatSnapshotsOf is ChatSnapshot for many chats, in order, read together:
	// one placement read for the whole list instead of one per chat.
	ChatSnapshotsOf(
		ctx context.Context,
		chatIDs []string,
	) ([]agentusecase.ChatSnapshot, error)

	// ListChatsByWorkspace returns every AgentChat anchored to workspaceID. List
	// calls this when its request still names a workspace (the home mount's
	// injected :wsId); otherwise it falls back to ListChats below.
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)

	// ListChatsInRepo returns every conversation-typed row whose cwd walk lands
	// on a workspace in repoID — what List answers once a request no longer
	// names a workspace (Task 17), and the enforcement of the repo boundary
	// that fallback originally went without. A row with no resolvable workspace
	// anywhere in its ancestry belongs to no repo, so it is dropped rather than
	// returned into whichever repo happened to ask.
	ListChatsInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)

	// CwdWorkspaceID answers where a row's CLI RUNS: the workspace of the
	// nearest ancestor-or-self carrying one (model spec §3.2). It is how a
	// BUBBLE, which carries no workspace of its own, is held to the mount it
	// was reached through — the same walk ListChatsInRepo scopes the list
	// beside it by, so Get and List cannot disagree. See chatGround.
	CwdWorkspaceID(
		ctx context.Context,
		chatID string,
	) (workspaceID string, ok bool, err error)

	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)

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
	// declared catalogue is refused with problem.ErrInvalidArgument (→ 400).
	SetChatSelection(
		ctx context.Context,
		chatID, model, effort string,
	) error

	// Promote fills a bubble's empty workspace slot: a worktree forked from the
	// chat's resolved fork parent, with its current provider respawned into it
	// (model spec §4.2). The chat keeps its id, title, placement and every turn
	// it has taken. It refuses a chat that already has a workspace
	// (ErrAlreadyPromoted), one with no workspace-owning ancestor to fork from
	// (ErrNoForkParent), and one that has never had a provider to respawn as
	// (ErrNothingToPromote).
	Promote(
		ctx context.Context,
		chatID string,
	) (domain.Chat, error)

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
	// Hook places one hook, over channel, and answers once its rows are
	// committed. A hook it cannot place is refused (404, 422) and dead-lettered.
	Hook(
		ctx context.Context,
		runnerID, provider, event string,
		raw []byte,
		channel engineagents.Channel,
	) (agentusecase.HookReply, error)

	// Telemetry is the provider's own report of cost and capacity, absent until
	// the provider makes one.
	Telemetry(chatID string) (engineagents.Telemetry, bool)
	// TelemetryOnChatSurface reports whether the report above can still be
	// TRUE: the store is durable, and a chat that moved to a surface whose
	// channel carries no usage would otherwise keep serving the last number
	// it earned somewhere else, forever.
	TelemetryOnChatSurface(ctx context.Context, chatID string) bool

	// UploadAttachment stores one attachment into chatID's durable attachment
	// store and returns the logical reference the caller writes back into the
	// message's own markdown text.
	UploadAttachment(
		ctx context.Context,
		chatID string,
		in agentusecase.UploadAttachmentInput,
	) (agentusecase.StoredAttachment, error)

	// ReadAttachment resolves chatID's stored attachment fileName to bytes and a
	// sniffed content type, or repoattachments.ErrNotFound.
	ReadAttachment(
		ctx context.Context,
		chatID, fileName string,
	) ([]byte, string, error)
}

// RunnerUsecase is the vendor CLI itself: which one is on a chat, what it has
// been told, and the lifecycle gestures a client can aim at it.
type RunnerUsecase interface {
	// provider/selection are the composer's STAGED pick, committed atomically
	// with the prompt. An empty provider is nothing staged, use the chat's
	// current one as-is; a non-empty one that differs from it is switched to
	// BEFORE the prompt is delivered — the frontend never calls a separate
	// switch endpoint for this any more, so there is exactly one path from a
	// picker row to a delivered prompt, not two. A nil selection is nothing
	// staged; a non-nil one is committed even when both halves are "" — see
	// domain.ChatSelection and agentusecase.Usecase.SubmitPrompt's own doc.
	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID, provider string,
		selection *domain.ChatSelection,
	) (domain.AgentPromptSubmission, error)

	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagents.SlashCatalog, error)

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

	// StopChat ends chatID's turn, in place where the provider can, else with its
	// CLI, leaving the chat dormant and resumable. No live runner: a no-op.
	StopChat(ctx context.Context, chatID string) error

	// Retract withdraws a sent prompt: settled while the provider has not taken
	// it, its turn interrupted once it has. A settled prompt is a conflict.
	Retract(ctx context.Context, chatID, requestID string) (agentusecase.RetractResult, error)

	// KillChat force-ends chatID's live vendor CLI, mid-turn included, and leaves
	// it dormant and resumable. Closing a view never ends it: that only detaches.
	KillChat(ctx context.Context, chatID string) error

	// Compact asks the chat's CLI to compact its own context. Crowbar cannot compact
	// anything itself — the context is the provider's — so this sends the provider's
	// own declared gesture, and 404s for a provider that declares none.
	Compact(
		ctx context.Context,
		chatID string,
	) error

	// SwitchToTerminal moves chatID onto its provider's own TUI and returns
	// the terminal session it is. Refuses (ErrTurnInProgress) while a turn is
	// in flight and (ErrNoNativeTerminal) for a provider with no TUI to show.
	SwitchToTerminal(
		ctx context.Context,
		chatID string,
	) (terminalSessionID string, err error)

	// SwitchToNative moves chatID onto Crowbar's own chat surface; a chat
	// already there is a no-op.
	SwitchToNative(
		ctx context.Context,
		chatID string,
	) error

	// AttachedTerminalSession answers which terminal session IS a runner's
	// native view right now, if it has one — see chatRuntime.
	AttachedTerminalSession(runnerID string) (string, bool)

	// TerminalWait is what the agent is blocked on that Crowbar CANNOT answer: a
	// modal no hook reports, read off the detector's standing answer so a REST
	// read never drives provider-screen work.
	TerminalWait(
		chatID string,
	) domain.AgentTerminalWait
}

// AnswerUsecase is the answer channel: the human deciding a prompt in the chat,
// and the in-PTY relay holding the provider's gate open until they do.
type AnswerUsecase interface {
	// AnswerTerminalWait types a descriptor-declared reply into the terminal
	// prompt the chat's CLI is parked on; ErrConflict once the prompt has gone.
	AnswerTerminalWait(ctx context.Context, chatID, answerID string) error

	// AnswerChoice records a human's decision and hands it to the relay blocking the
	// provider's gate. optionIDs name what was picked; reason and content carry the
	// free-form parts a provider accepts (a deny message, an elicitation's filled-in
	// form). It fails with problem.ErrConflict when no relay is waiting, because an
	// answer that reaches nobody must never be reported as delivered.
	AnswerChoice(
		ctx context.Context,
		chatID, choiceID string,
		optionIDs []string,
		reason string,
		content []byte,
	) error

	// HoldsAnswer reports whether askID is on the desk: still open, or decided with a
	// verdict its hook has not collected yet; either way the hook must be held.
	HoldsAnswer(askID string) bool

	// AwaitAnswer blocks that hook until its prompt is decided, the budget expires,
	// or the request is cancelled. Only the first carries bytes to print.
	AwaitAnswer(
		ctx context.Context,
		askID string,
	) (agentusecase.HookAnswer, error)

	// SetChatPermissionLevel overrides chatID's trust dial for the rest of its
	// lifetime, independent of the global default (see
	// ProviderUsecase.DefaultPermissionLevel). Refuses a level the chat's
	// current provider does not declare, with problem.ErrInvalidArgument (→ 400).
	SetChatPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) error
}

// ProviderUsecase is the global provider table and the MCP transport the vendor
// CLIs call back into. Nothing here is workspace-scoped.
type ProviderUsecase interface {
	InstallDescriptor(context.Context, []byte) (descriptorcheck.Report, error)
	// ResolveProviders returns the enriched, priority-ordered provider list the
	// backend owns: the descriptor catalog joined with the global preference table
	// and the install probe (connected + enabled, in priority order). It takes no
	// wsId — providers are global — and backs the enriched GET .../chats/providers.
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	// DescriptorReports validates every descriptor this machine would load; it
	// backs GET /v0/settings/chat/descriptors.
	DescriptorReports(
		ctx context.Context,
	) ([]descriptorcheck.Report, error)

	// ReplaceProviderPreferences rewrites the whole global preference table from the
	// submitted ordered set (array position → priority), validating ids against the
	// catalog (unknown → problem.ErrInvalidArgument → 400) and returning the freshly
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

	// VerifyRunnerToken reports whether token is the one minted for runnerID: the
	// credential a vendor CLI's HTTP hook proves itself with.
	VerifyRunnerToken(runnerID, token string) bool

	// DefaultPermissionLevel and SetDefaultPermissionLevel back
	// GET/PUT /v0/settings/chat/permission-level.
	DefaultPermissionLevel(
		ctx context.Context,
	) (string, error)
	SetDefaultPermissionLevel(
		ctx context.Context,
		level string,
	) error

	// ModelManifestFetchEnabled and SetModelManifestFetchEnabled back
	// GET/PUT /v0/settings/chat/model-manifest-fetch — whether a
	// model.manifest: source's background refresh may hit the network.
	ModelManifestFetchEnabled(
		ctx context.Context,
	) (bool, error)
	SetModelManifestFetchEnabled(
		ctx context.Context,
		enabled bool,
	) error
}
