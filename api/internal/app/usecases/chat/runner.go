package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// RunnerUsecase owns the vendor CLI: start, replace, resume, stop, prompt. Every path
// that starts or ends a CLI decides under the chat's hook lock (so two switches never
// put two CLIs on one chat) and waits with it released, so hooks are never held up.
type RunnerUsecase interface {
	// SpawnChat mints a chat and starts a CLI on it in one act, returning both
	// ids. A refused spawn takes the half-created chat with it.
	SpawnChat(
		ctx context.Context,
		workspaceID string,
		providerID string,
	) (chatID, runnerID string, err error)

	// StartRunner starts a CLI on a chat that already exists.
	StartRunner(
		ctx context.Context,
		chatID string,
		providerID string,
	) (string, error)

	// ResumeChat revives a dormant chat on the provider that last held it,
	// returning the id of the runner now on it. A chat that already has a live
	// runner is returned unchanged.
	ResumeChat(
		ctx context.Context,
		chatID string,
	) (string, error)

	// StopChat quits the CLI on a chat, leaving the chat dormant. A chat with no
	// live runner is already stopped.
	StopChat(
		ctx context.Context,
		chatID string,
	) error

	// Retract withdraws a sent prompt: settled while no CLI holds it, its turn
	// interrupted once it runs. A prompt a CLI holds, or a settled one, is a conflict.
	Retract(ctx context.Context, chatID, requestID string) (RetractResult, error)

	// KillChat force-ends the CLI on a chat, even mid-turn, leaving it dormant and
	// resumable. It is the only way a client ends a running agent: closing a view
	// only detaches.
	KillChat(
		ctx context.Context,
		chatID string,
	) error

	// SwitchProvider replaces the chat's CLI with another provider's, handing over the
	// conversation. It WAITS out an in-flight turn, and refuses a disabled target
	// before anything is torn down.
	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
	) (string, error)

	// SubmitPrompt delivers a prompt at most once per clientRequestID (a retry replays the
	// outcome). provider/selection are the composer's STAGED pick, applied atomically
	// with the send: "" / nil keep the current; an all-empty selection resets to default.
	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID, provider string,
		selection *domain.ChatSelection,
	) (domain.AgentPromptSubmission, error)

	// SlashCatalog probes the chat's live CLI for the slash commands it declares.
	// A probe superseded by a newer one on the same chat is refused rather than
	// answered late.
	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagents.SlashCatalog, error)

	// SwitchToTerminal moves the chat onto its provider's own TUI — idle-only
	// when that relaunches or hands over a process — and returns the terminal
	// session the TUI is ("" for a dormant chat, which only records the move).
	SwitchToTerminal(
		ctx context.Context,
		chatID string,
	) (string, error)

	// SwitchToNative moves the chat onto Crowbar's own chat surface. A chat
	// already there is a no-op.
	SwitchToNative(
		ctx context.Context,
		chatID string,
	) error

	// AttachedTerminalSession answers which terminal session IS a live runner's view,
	// set only between SwitchToTerminal and SwitchToNative; false when none is attached.
	AttachedTerminalSession(runnerID string) (string, bool)

	// TelemetryOnChatSurface is the CHAT-scoped form of a capability a
	// provider-scoped flag cannot answer: two chats of the same provider can
	// be on different surfaces. See runner/capabilities.go.
	TelemetryOnChatSurface(ctx context.Context, chatID string) bool

	// ShutdownAPIConnections notes daemon_restart on every live runner and
	// kills every api-transport connection this daemon still holds: the
	// shutdown-time counterpart to ReconcileRunnersOnBoot.
	ShutdownAPIConnections()

	// LiveRunnerForChat returns the runner on a chat; ErrNoLiveRunner when it
	// is dormant.
	LiveRunnerForChat(
		ctx context.Context,
		chatID string,
	) (engineagents.Runner, error)

	// ConversationsForChat lists every provider conversation ever bound to a chat,
	// oldest first.
	ConversationsForChat(
		ctx context.Context,
		chatID string,
	) ([]engineagents.ChatConversation, error)

	// ReconcileRunnersOnBoot records daemon_restart for every runner the ledger
	// still holds live, closes the turns they died in, and settles the prompts
	// they left pending.
	ReconcileRunnersOnBoot(
		ctx context.Context,
	) error

	// RestateProvidersFromHistory makes each chat's durable provider agree with
	// newest, the provider the legacy history last names per chat. It runs once
	// per install and counts the rows it skipped.
	RestateProvidersFromHistory(
		ctx context.Context,
		newest map[string]string,
	) (skipped int, err error)

	// StaleProviders lists the replayable chats RestateProvidersFromHistory left
	// on a provider newest disagrees with.
	StaleProviders(
		ctx context.Context,
		newest map[string]string,
	) ([]string, error)

	// Compact asks the chat's CLI to compact its own context, using the gesture the
	// provider declares. A provider that declares none reports ErrNotFound.
	Compact(
		ctx context.Context,
		chatID string,
	) error

	// TerminalWait reports whether a chat's CLI is parked on a modal terminal
	// prompt, and on which kind. A daemon whose terminal seam cannot render a
	// screen always answers "not waiting".
	TerminalWait(
		chatID string,
	) domain.AgentTerminalWait

	// StartTerminalWaitSweep starts the screen sweep and binds the four publish
	// callbacks the hub owns. It runs until ctx is cancelled.
	StartTerminalWaitSweep(ctx context.Context, feed ChatFeed)
}

var _ RunnerUsecase = (*Usecase)(nil)

// RetractResult is what a retract did ("retracted" | "interrupted") and the
// prompt's text, handed back to the composer.
type RetractResult = runner.RetractResult

// The CLI lifecycle's refusals, re-exported so a handler can map them to a status
// without importing past the door. Each is the SAME value the lifecycle returns,
// so errors.Is matches across the boundary.
var (
	ErrSlashCatalogUnsupported = runner.ErrSlashCatalogUnsupported
	ErrSlashCatalogTimeout     = runner.ErrSlashCatalogTimeout
	ErrSlashCatalogUnavailable = runner.ErrSlashCatalogUnavailable
	ErrSlashCatalogOutputLimit = runner.ErrSlashCatalogOutputLimit
	ErrSlashCatalogCommand     = runner.ErrSlashCatalogCommand
	ErrSlashCatalogMalformed   = runner.ErrSlashCatalogMalformed
	ErrSlashCatalogSuperseded  = runner.ErrSlashCatalogSuperseded

	// ErrProviderExitedDuringStartup is a vendor CLI that died before its runner
	// row could even be persisted — a dependency the user can act on, not a bug.
	ErrProviderExitedDuringStartup = runner.ErrProviderExitedDuringStartup

	ErrPromptBusy               = runner.ErrPromptBusy
	ErrPromptRequestIDConflict  = runner.ErrPromptRequestIDConflict
	ErrPromptOutcomeUnknown     = runner.ErrPromptOutcomeUnknown
	ErrPromptUnsupported        = runner.ErrPromptUnsupported
	ErrPromptSessionUnavailable = runner.ErrPromptSessionUnavailable
)

// SpawnChat mints a chat and starts a provider CLI on it in one step.
func (u *Usecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	return u.runners.SpawnChat(ctx, workspaceID, providerID)
}

// StartRunner starts a provider CLI on a chat that already exists.
func (u *Usecase) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	return u.runners.StartRunner(ctx, chatID, providerID)
}

// ResumeChat restarts the chat's own provider on it, resuming the conversation
// the previous CLI left behind.
func (u *Usecase) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.runners.ResumeChat(ctx, chatID)
}

// StopChat quits the CLI on a chat. The chat itself survives.
func (u *Usecase) StopChat(
	ctx context.Context,
	chatID string,
) error {
	return u.runners.StopChat(ctx, chatID)
}

// Retract withdraws requestID from the chat; its text comes back.
func (u *Usecase) Retract(ctx context.Context, chatID, requestID string) (RetractResult, error) {
	return u.runners.Retract(ctx, chatID, requestID)
}

// KillChat force-ends the CLI on a chat. The chat itself survives.
func (u *Usecase) KillChat(
	ctx context.Context,
	chatID string,
) error {
	return u.runners.KillChat(ctx, chatID)
}

// SwitchProvider replaces the chat's CLI with another provider's, waiting out any turn
// in flight. A same-provider switch is deliberately NOT a no-op: it is Promote's
// respawn primitive, rebuilding a CLI whose worktree just changed.
func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	return u.runners.SwitchProvider(ctx, chatID, targetProviderID)
}

// SubmitPrompt delivers a prompt at most once per client request id, in one hold
// of the chat. A staged provider unlike the chat's is switched to by the send's
// one launch; a non-nil selection, validated first, rides the send.
func (u *Usecase) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID, provider string,
	selection *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	return u.runners.SubmitPromptWithSwitch(ctx, chatID, text, clientRequestID, provider, selection)
}

// SlashCatalog probes the chat's provider for the slash commands it offers.
// Results are deliberately never cached: a catalog changes when the user edits
// their own command files.
func (u *Usecase) SlashCatalog(
	ctx context.Context,
	chatID string,
) (engineagents.SlashCatalog, error) {
	return u.runners.SlashCatalog(ctx, chatID)
}

// ReconcileRunnersOnBoot ends every runner whose PTY did not survive the
// restart. Nothing else can: no exit was ever recorded for a process the daemon
// outlived.
func (u *Usecase) ReconcileRunnersOnBoot(
	ctx context.Context,
) error {
	return u.runners.ReconcileRunnersOnBoot(ctx)
}

// RestateProvidersFromHistory is the one-off upgrade of every chat's durable
// provider to what its legacy history names; see Conversations'.
func (u *Usecase) RestateProvidersFromHistory(
	ctx context.Context,
	newest map[string]string,
) (int, error) {
	return u.conversations.RestateProvidersFromHistory(ctx, newest)
}

// StaleProviders implements RunnerUsecase.
func (u *Usecase) StaleProviders(
	ctx context.Context,
	newest map[string]string,
) ([]string, error) {
	return u.conversations.StaleProviders(ctx, newest)
}

// Compact asks the chat's provider to compact its own context, through whichever
// gesture the provider's descriptor declares for it.
func (u *Usecase) Compact(ctx context.Context, chatID string) error {
	// The one place a manual request is knowable; armed even if the call below then
	// fails validation (see manualCompactRequests).
	u.recorder.ArmManualCompaction(chatID)
	return u.runners.Compact(ctx, chatID)
}

// SwitchToTerminal moves the chat onto its provider's own TUI.
func (u *Usecase) SwitchToTerminal(ctx context.Context, chatID string) (string, error) {
	return u.runners.SwitchToTerminal(ctx, chatID)
}

// SwitchToNative moves the chat onto Crowbar's own chat surface.
func (u *Usecase) SwitchToNative(ctx context.Context, chatID string) error {
	return u.runners.SwitchToNative(ctx, chatID)
}

// AttachedTerminalSession answers which terminal session IS a runner's
// native view right now, if it has one.
func (u *Usecase) AttachedTerminalSession(runnerID string) (string, bool) {
	return u.runners.AttachedTerminalSession(runnerID)
}

// TelemetryOnChatSurface reports whether this chat can still receive usage
// reports on the surface it is on now.
func (u *Usecase) TelemetryOnChatSurface(ctx context.Context, chatID string) bool {
	return u.runners.TelemetryOnChatSurface(ctx, chatID)
}

// ShutdownAPIConnections notes daemon_restart on every live runner and kills
// every api-transport connection this daemon still holds.
func (u *Usecase) ShutdownAPIConnections() {
	u.runners.Shutdown()
}

// TerminalWait reports whether the chat's CLI is parked on a modal its own
// terminal is showing — the state no hook reports.
func (u *Usecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	return u.runners.TerminalWait(chatID)
}

// StartTerminalWaitSweep starts the screen sweep and wires the publish callbacks
// the hub owns.
func (u *Usecase) StartTerminalWaitSweep(ctx context.Context, feed ChatFeed) {
	u.feed.Store(&feed)
	u.runners.StartTerminalWaitSweep(ctx)
}

// OpenChat is a just-created chat's launch: a CLI now only on the terminal.
func (u *Usecase) OpenChat(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	return u.runners.OpenChat(ctx, chatID, providerID)
}
