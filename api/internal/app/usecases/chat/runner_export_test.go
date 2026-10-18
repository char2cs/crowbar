package chat

import (
	"context"
	"fmt"

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

// ChatSession is how chatID's last runner ended, as its snapshot reports it.
func ChatSession(u RunnerUsecase, chatID string) domain.AgentSession {
	snap, err := concrete(u).ChatSnapshot(context.Background(), chatID)
	if err != nil {
		panic(err)
	}
	return snap.Session
}

// concrete is u at the type every test builds; any other is a test bug.
func concrete(u RunnerUsecase) *Usecase {
	uc, ok := u.(*Usecase)
	if !ok {
		panic(fmt.Sprintf("chat: %T is not the chat usecase", u))
	}
	return uc
}

// AllLiveRunners is every started runner the supervisor holds.
func AllLiveRunners(u *Usecase) []engineagents.Runner {
	all, err := u.runners.AllLive(context.Background())
	if err != nil {
		panic(err)
	}
	return all
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
