package chat

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// WaitingForPendingPromptLog is logged the instant a provider switch parks on a
// prompt the provider has not acknowledged, so a test can block on it.
const WaitingForPendingPromptLog = runner.WaitingForPendingPromptLog

// HasPendingPrompt reports whether chatID holds a prompt the provider has not
// acknowledged yet, read straight off the ledger.
func HasPendingPrompt(u RunnerUsecase, ctx context.Context, chatID string) bool {
	_, ok, err := u.PendingPrompt(ctx, chatID)
	return err == nil && ok
}

// ChatSession is the session supervisor's record for chatID: the rung its
// runner launched on and how its last runner ended.
func ChatSession(u RunnerUsecase, chatID string) domain.AgentSession {
	uc, ok := u.(*Usecase)
	if !ok {
		return domain.AgentSession{}
	}
	return uc.runners.Session(chatID)
}

// SetSwitchAwaitTimeout overrides how long a provider switch waits for the
// outgoing turn to finish naturally before forcing it (see forceSwitchAfter).
// Test-only surface: production always uses termwait.DefaultStallQuiet.
func SetSwitchAwaitTimeout(u RunnerUsecase, d time.Duration) {
	if uc, ok := u.(*Usecase); ok {
		uc.runners.SetSwitchAwaitTimeout(d)
	}
}

// RequirePromptRestart exposes the delivery guard SubmitPrompt runs before it
// touches anything: no loadable descriptor reaches its refusal branch, so only a
// descriptor built in Go can exercise it.
func RequirePromptRestart(
	ctx context.Context,
	u RunnerUsecase,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) error {
	return u.(*Usecase).runners.RequirePromptRestart(ctx, chatID, live, descriptor)
}
