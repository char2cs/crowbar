package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// SetPromptJournalDirSync installs a deterministic durability fault for external
// package tests. It is test-only surface; production always uses fsync+close on
// the journal parent directory after the atomic rename.
//
// It REPLACES the journal, so it must be called before the chat under test has
// submitted anything. The prompt journal holds no state between calls, so a
// replacement loses nothing.
func SetPromptJournalDirSync(u RunnerUsecase, syncDir func(string) error) {
	u.(*Usecase).runners.SetPromptJournal(agentjournal.NewPromptRequests(agentjournal.WithDirSync(syncDir)))
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
func RequirePromptRestart(
	ctx context.Context,
	u RunnerUsecase,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) error {
	return u.(*Usecase).runners.RequirePromptRestart(ctx, chatID, live, descriptor)
}
