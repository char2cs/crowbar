package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type RunnerUsecase interface {
	StartRunner(
		ctx context.Context,
		chatID string,
		providerID string,
	) (string, error)

	ResumeChat(
		ctx context.Context,
		chatID string,
	) (string, error)

	StopChat(
		ctx context.Context,
		chatID string,
	) error

	Retract(ctx context.Context, chatID, requestID string) (RetractResult, error)

	KillChat(
		ctx context.Context,
		chatID string,
	) error

	SwitchProvider(
		ctx context.Context,
		chatID string,
		targetProviderID string,
		surface string,
	) (string, error)

	SubmitPrompt(
		ctx context.Context,
		chatID, text, clientRequestID, provider string,
		selection *domain.ChatSelection,
	) (domain.AgentPromptSubmission, error)

	SlashCatalog(
		ctx context.Context,
		chatID string,
	) (engineagents.SlashCatalog, error)

	SwitchToTerminal(
		ctx context.Context,
		chatID string,
	) (string, error)

	SwitchToNative(
		ctx context.Context,
		chatID string,
	) error

	AttachedTerminalSession(runnerID string) (string, bool)

	TelemetryOnChatSurface(ctx context.Context, chatID string) bool

	ShutdownAPIConnections()

	LiveRunnerForChat(
		ctx context.Context,
		chatID string,
	) (engineagents.Runner, error)

	ConversationsForChat(
		ctx context.Context,
		chatID string,
	) ([]engineagents.ChatConversation, error)

	ReconcileRunnersOnBoot(
		ctx context.Context,
	) error

	RestateProvidersFromHistory(
		ctx context.Context,
		newest map[string]string,
	) (skipped int, err error)

	StaleProviders(
		ctx context.Context,
		newest map[string]string,
	) ([]string, error)

	Compact(
		ctx context.Context,
		chatID string,
	) error

	StopShell(
		ctx context.Context,
		chatID, shellID string,
	) error

	TerminalWait(
		chatID string,
	) domain.AgentTerminalWait

	StartTerminalWaitSweep(ctx context.Context, feed ChatFeed)
}

var _ RunnerUsecase = (*Usecase)(nil)

type RetractResult = runner.RetractResult

var (
	ErrSlashCatalogUnsupported = runner.ErrSlashCatalogUnsupported
	ErrSlashCatalogTimeout     = runner.ErrSlashCatalogTimeout
	ErrSlashCatalogUnavailable = runner.ErrSlashCatalogUnavailable
	ErrSlashCatalogOutputLimit = runner.ErrSlashCatalogOutputLimit
	ErrSlashCatalogCommand     = runner.ErrSlashCatalogCommand
	ErrSlashCatalogMalformed   = runner.ErrSlashCatalogMalformed
	ErrSlashCatalogSuperseded  = runner.ErrSlashCatalogSuperseded

	ErrProviderExitedDuringStartup = runner.ErrProviderExitedDuringStartup

	ErrPromptBusy               = runner.ErrPromptBusy
	ErrPromptRequestIDConflict  = runner.ErrPromptRequestIDConflict
	ErrPromptOutcomeUnknown     = runner.ErrPromptOutcomeUnknown
	ErrPromptUnsupported        = runner.ErrPromptUnsupported
	ErrPromptSessionUnavailable = runner.ErrPromptSessionUnavailable
)

func (u *Usecase) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	return u.runners.StartRunner(ctx, chatID, providerID)
}

func (u *Usecase) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.runners.ResumeChat(ctx, chatID)
}

func (u *Usecase) StopChat(
	ctx context.Context,
	chatID string,
) error {
	return u.runners.StopChat(ctx, chatID)
}

func (u *Usecase) Retract(ctx context.Context, chatID, requestID string) (RetractResult, error) {
	return u.runners.Retract(ctx, chatID, requestID)
}

func (u *Usecase) KillChat(
	ctx context.Context,
	chatID string,
) error {
	return u.runners.KillChat(ctx, chatID)
}

func (u *Usecase) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
	surface string,
) (string, error) {
	return u.runners.SwitchProvider(ctx, chatID, targetProviderID, surface)
}

func (u *Usecase) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID, provider string,
	selection *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	return u.runners.SubmitPromptWithSwitch(ctx, chatID, text, clientRequestID, provider, selection)
}

func (u *Usecase) SlashCatalog(
	ctx context.Context,
	chatID string,
) (engineagents.SlashCatalog, error) {
	return u.runners.SlashCatalog(ctx, chatID)
}

func (u *Usecase) ReconcileRunnersOnBoot(
	ctx context.Context,
) error {
	return u.runners.ReconcileRunnersOnBoot(ctx)
}

func (u *Usecase) RestateProvidersFromHistory(
	ctx context.Context,
	newest map[string]string,
) (int, error) {
	return u.conversations.RestateProvidersFromHistory(ctx, newest)
}

func (u *Usecase) StaleProviders(
	ctx context.Context,
	newest map[string]string,
) ([]string, error) {
	return u.conversations.StaleProviders(ctx, newest)
}

func (u *Usecase) Compact(ctx context.Context, chatID string) error {
	u.recorder.ArmManualCompaction(chatID)
	return u.runners.Compact(ctx, chatID)
}

func (u *Usecase) StopShell(ctx context.Context, chatID, shellID string) error {
	return u.runners.StopShell(ctx, chatID, shellID)
}

func (u *Usecase) SwitchToTerminal(ctx context.Context, chatID string) (string, error) {
	return u.runners.SwitchToTerminal(ctx, chatID)
}

func (u *Usecase) SwitchToNative(ctx context.Context, chatID string) error {
	return u.runners.SwitchToNative(ctx, chatID)
}

func (u *Usecase) AttachedTerminalSession(runnerID string) (string, bool) {
	return u.runners.AttachedTerminalSession(runnerID)
}

func (u *Usecase) TelemetryOnChatSurface(ctx context.Context, chatID string) bool {
	return u.runners.TelemetryOnChatSurface(ctx, chatID)
}

func (u *Usecase) ShutdownAPIConnections() {
	u.runners.Shutdown()
}

func (u *Usecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	return u.runners.TerminalWait(chatID)
}

func (u *Usecase) StartTerminalWaitSweep(ctx context.Context, feed ChatFeed) {
	u.feed.Store(&feed)
	u.runners.StartTerminalWaitSweep(ctx)
}

func (u *Usecase) OpenChat(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	return u.runners.OpenChat(ctx, chatID, providerID)
}
