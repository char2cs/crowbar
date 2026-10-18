package chat

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// HasPendingPrompt reports whether chatID holds a prompt the provider has not
// acknowledged yet, read straight off the ledger.
func HasPendingPrompt(u RunnerUsecase, ctx context.Context, chatID string) bool {
	_, ok, err := u.PendingPrompt(ctx, chatID)
	return err == nil && ok
}

// RequirePromptRestart exposes the delivery guard SubmitPrompt runs before it
// touches anything. It is test-only surface: this file is compiled only under
// `go test`.
//
// It is exposed because the guard's refusal branch has no descriptor that can
// reach it. A strategy this daemon cannot drive is refused at LOAD by the
// descriptor rules, so the only way to ask the guard about one is to hand it a
// descriptor built in Go — which is the point: the day a strategy is made
// declarable before it is made deliverable, this is the lock that still holds,
// and a lock nothing exercises is a lock nobody notices going missing.
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
	u.(*Usecase).runners.SetSwitchAwaitTimeout(d)
}

func RequirePromptRestart(
	ctx context.Context,
	u RunnerUsecase,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) error {
	return u.(*Usecase).runners.RequirePromptRestart(ctx, chatID, live, descriptor)
}
