package handlers

import (
	"context"
	"io"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// ChatUsecase is the chat aggregate as the handlers need it: the listed rows, one chat's record, and
// the writes that retitle it or pin its next CLI's model. It starts no processes, so every route served
// off it answers whether or not a runner was ever placed on the chat.
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

	// ListChatsInRepo returns every conversation row whose cwd walk lands on a workspace in repoID. A row
	// with no resolvable workspace in its ancestry belongs to no repo and is dropped, not leaked into
	// whichever repo asked.
	ListChatsInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)

	// CwdWorkspaceID is the workspace a row's CLI runs in: that of its nearest ancestor-or-self carrying
	// one. It is the same walk ListChatsInRepo scopes by, so Get and List cannot disagree on a bubble.
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

	// PurgeChat hard-deletes chatID, gone from every read once it returns, then best-effort kills its
	// CLI. No route calls it directly: DELETE goes through ChatTreeUsecase, which knows the chat's threads.
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
	// TelemetryOnChatSurface reports whether Telemetry can still be true: the store is durable, so a chat
	// moved to a surface whose channel carries no usage would otherwise serve a stale number forever.
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

	// BeginAttachmentUpload starts or resumes a chunked upload and answers the bytes already held.
	BeginAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (int64, error)
	// AppendAttachmentUpload writes one chunk at offset and answers the bytes held afterwards.
	AppendAttachmentUpload(ctx context.Context, chatID, id string, offset int64, chunk io.Reader) (int64, error)
	// CompleteAttachmentUpload stores the staged bytes as the attachment UploadAttachment would.
	CompleteAttachmentUpload(ctx context.Context, chatID, id, fileName string, size int64) (agentusecase.StoredAttachment, error)
	// AbortAttachmentUpload deletes a chunked upload's staged bytes.
	AbortAttachmentUpload(ctx context.Context, chatID, id string) error
}

// RunnerUsecase is the vendor CLI itself: which one is on a chat, what it has
// been told, and the lifecycle gestures a client can aim at it.
type RunnerUsecase interface {
	// SubmitPrompt delivers text, committing the composer's staged pick atomically with it: a non-empty
	// provider that differs from the current one is switched to first, and a non-nil selection is
	// committed even when both halves are empty (see domain.ChatSelection).
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
	// returning the new runner's id. A non-empty surface is where the new runner starts.
	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
		surface string,
	) (newRunnerID string, err error)

	// ResumeChat revives a dormant chat (its CLI exited or died with the daemon) into the provider and
	// conversation it left. A still-live chat is a no-op returning its current runner's id.
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

	// StopShell asks chatID's agent to end its running background shell shellID by the
	// words its provider declares (ErrNotFound when it declares none) and marks the
	// shell stopping; the shell closes only when the agent ends it.
	StopShell(ctx context.Context, chatID, shellID string) error

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

	// AnswerChoice hands a human's decision to the relay blocking the provider's gate. It fails with
	// problem.ErrConflict when no relay is waiting: an answer that reaches nobody is never "delivered".
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

	// SetChatPermissionLevel overrides chatID's trust dial independently of the global default, refusing
	// (problem.ErrInvalidArgument) a level the chat's current provider does not declare.
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
	// ResolveProviders returns the global, priority-ordered provider list: the descriptor catalog joined
	// with the preference table and the install probe.
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	// DescriptorReports validates every descriptor this machine would load; it
	// backs GET /v0/settings/chat/descriptors.
	DescriptorReports(
		ctx context.Context,
	) ([]descriptorcheck.Report, error)

	// ReplaceProviderPreferences rewrites the global preference table from the ordered set (position is
	// priority), refusing unknown ids with problem.ErrInvalidArgument, and returns the resolved list.
	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]domain.AgentProvider, error)

	// DispatchMCP runs one raw MCP JSON-RPC message for runnerID, authenticated by token; the framing is
	// the engine's business, so the handler only carries bytes. The bool is false for a notification,
	// which gets no reply.
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
