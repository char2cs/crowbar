package chat

import (
	"context"
	"errors"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// RunnerUsecase owns the vendor CLI: starting one on a chat, replacing it,
// resuming it, stopping it, and delivering a React-authored prompt to it.
//
// Every path in here that starts or ends a CLI on a chat holds that chat's
// spawn gate, which is the only thing that can stop two concurrent switches
// putting two CLIs on one chat (see inflight.Gate). The gate is NEVER taken on the
// hook path, and the destructive paths park on the in-flight turn instead of
// decapitating it.
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

	// SwitchProvider replaces the chat's CLI with one from another provider,
	// handing the incoming CLI the conversation so far. It WAITS for any in-flight
	// turn rather than quitting a CLI mid-answer, and refuses a disabled target
	// before anything is torn down.
	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
	) (string, error)

	// SubmitPrompt delivers a React-authored prompt to the chat's CLI. Delivery is
	// at-most-once against clientRequestID: a retry replays the original outcome
	// rather than prompting twice.
	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID string,
	) (domain.AgentPromptSubmission, error)

	// SlashCatalog probes the chat's live CLI for the slash commands it declares.
	// A probe superseded by a newer one on the same chat is refused rather than
	// answered late.
	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagents.SlashCatalog, error)

	// LiveRunnerForChat returns the runner currently placed on a chat.
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

	// ReconcileRunnersOnBoot Exits every recorded runner whose PTY did not survive
	// the restart, closes the turns they died in, and recovers their prompt
	// journals.
	ReconcileRunnersOnBoot(
		ctx context.Context,
	) error

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

	// StartTerminalWaitSweep starts the screen sweep and binds the three publish
	// callbacks the hub owns. It runs until ctx is cancelled.
	StartTerminalWaitSweep(
		ctx context.Context,
		publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
		promptSettled func(chatID, workspaceID, requestID string),
		messageDelta func(chatID, workspaceID, messageID, text string),
	)
}

var _ RunnerUsecase = (*Usecase)(nil)

// The CLI lifecycle's refusals, re-exported so a handler can map them to a status
// without importing past the door. Each is the SAME value the lifecycle returns,
// so errors.Is matches across the boundary.
var (
	ErrSlashCatalogUnsupported = runner.ErrSlashCatalogUnsupported
	ErrSlashCatalogNoLiveTUI   = runner.ErrSlashCatalogNoLiveTUI
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
	ErrPromptAlreadyAccepted    = runner.ErrPromptAlreadyAccepted
	ErrPromptUnsupported        = runner.ErrPromptUnsupported
	ErrPromptSessionUnavailable = runner.ErrPromptSessionUnavailable
)

const (
	PromptCodeBusy              = "chat_busy"
	PromptCodeOutcomeUnknown    = "request_outcome_uncertain"
	PromptCodeAlreadyAccepted   = "request_already_accepted"
	PromptCodeRequestIDConflict = "request_id_conflict"
	PromptCodeUnsupported       = "prompt_submit_unsupported"
	PromptCodeSessionRequired   = "live_tui_required"
)

const (
	CatalogCodeUnsupported  = "catalog_unsupported"
	CatalogCodeLiveRequired = "catalog_live_tui_required"
	CatalogCodeTimeout      = "catalog_timeout"
	CatalogCodeUnavailable  = "catalog_command_unavailable"
	CatalogCodeOutputLimit  = "catalog_output_limit"
	CatalogCodeCommand      = "catalog_command_failed"
	CatalogCodeMalformed    = "catalog_malformed_output"
	CatalogCodeSuperseded   = "catalog_superseded"
)

// PromptErrorCode returns the stable machine-readable API code for a prompt
// submission error. The human message may evolve; clients branch only on this.
func PromptErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPromptBusy):
		return PromptCodeBusy
	case errors.Is(err, ErrPromptOutcomeUnknown):
		return PromptCodeOutcomeUnknown
	case errors.Is(err, ErrPromptAlreadyAccepted):
		return PromptCodeAlreadyAccepted
	case errors.Is(err, ErrPromptRequestIDConflict):
		return PromptCodeRequestIDConflict
	case errors.Is(err, ErrPromptUnsupported):
		return PromptCodeUnsupported
	case errors.Is(err, ErrPromptSessionUnavailable):
		return PromptCodeSessionRequired
	default:
		return ""
	}
}

func CatalogErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrSlashCatalogUnsupported):
		return CatalogCodeUnsupported
	case errors.Is(err, ErrSlashCatalogNoLiveTUI):
		return CatalogCodeLiveRequired
	case errors.Is(err, ErrSlashCatalogTimeout):
		return CatalogCodeTimeout
	case errors.Is(err, ErrSlashCatalogUnavailable):
		return CatalogCodeUnavailable
	case errors.Is(err, ErrSlashCatalogOutputLimit):
		return CatalogCodeOutputLimit
	case errors.Is(err, ErrSlashCatalogCommand):
		return CatalogCodeCommand
	case errors.Is(err, ErrSlashCatalogMalformed):
		return CatalogCodeMalformed
	case errors.Is(err, ErrSlashCatalogSuperseded):
		return CatalogCodeSuperseded
	default:
		return ""
	}
}

// The vendor CLI's lifecycle. Everything here is USER-INITIATED, which is what
// separates it from the hook ingress: it may block, it may refuse, and it takes
// the per-chat spawn gate — the only thing that stops two clicks putting two CLIs
// on one chat.

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

// SwitchProvider replaces the chat's CLI with another provider's, waiting for any
// turn in flight so the outgoing CLI is never killed mid-answer.
func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	return u.runners.SwitchProvider(ctx, chatID, targetProviderID)
}

// SubmitPrompt delivers a Crowbar-authored prompt into the chat's CLI, at most
// once per client request id.
func (u *Usecase) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID string,
) (domain.AgentPromptSubmission, error) {
	return u.runners.SubmitPrompt(ctx, chatID, text, clientRequestID)
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

// LiveRunnerForChat returns the CLI currently placed on the chat.
func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (engineagents.Runner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

// ConversationsForChat returns every conversation a CLI has hosted on the chat.
func (u *Usecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]engineagents.ChatConversation, error) {
	return u.runners.ConversationsForChat(ctx, chatID)
}

// ReconcileRunnersOnBoot exits every runner whose PTY did not survive the
// restart. Nothing else can: no event was ever recorded for a process the daemon
// outlived.
func (u *Usecase) ReconcileRunnersOnBoot(
	ctx context.Context,
) error {
	return u.runners.ReconcileRunnersOnBoot(ctx)
}

// Compact asks the chat's provider to compact its own context, through whichever
// gesture the provider's descriptor declares for it.
func (u *Usecase) Compact(ctx context.Context, chatID string) error {
	return u.runners.Compact(ctx, chatID)
}

// TerminalWait reports whether the chat's CLI is parked on a modal its own
// terminal is showing — the state no hook reports.
func (u *Usecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	return u.runners.TerminalWait(chatID)
}

// StartTerminalWaitSweep starts the screen sweep and wires the publish callbacks
// the hub owns.
func (u *Usecase) StartTerminalWaitSweep(
	ctx context.Context,
	publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
	promptSettled func(chatID, workspaceID, requestID string),
	messageDelta func(chatID, workspaceID, messageID, text string),
) {
	u.runners.StartTerminalWaitSweep(ctx, publish, promptSettled, messageDelta)
}
