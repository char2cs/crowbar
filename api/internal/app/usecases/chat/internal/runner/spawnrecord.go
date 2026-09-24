// Package runner (file spawnrecord.go) commits a spawned CLI to the durable
// record: the chat row a create has to mint first, the runner row naming the
// process that was just started, and the eviction any PLACEMENT implies.
//
// Split from spawn.go, which orchestrates the spawn itself — deciding what to
// run and starting it — once that file outgrew being readable in one sitting.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (rs *Runners) recordRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	runnerID string,
	termSessID string,
	launchSessionID string,
	sel engineagents.Selection,
	create bool,
) error {
	now := time.Now()
	if create {
		created, err := rs.chats.Create(ctx, agentchat.CreateInput{
			ID:          chatID,
			WorkspaceID: workspaceID,
			Type:        domain.ChatTypeChat,
			ProviderID:  providerID,
			Now:         now,
		})
		if err != nil {
			return rs.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
				fmt.Errorf("agent: spawn runner: create chat: %w", err))
		}
		rs.work.Set(chatID, created.Working)
		rs.seedPermissionLevel(ctx, chatID)
	}
	if _, err := rs.runnerStore.Start(ctx, agentrunner.StartInput{
		RunnerID:        runnerID,
		WorkspaceID:     workspaceID,
		ProviderID:      providerID,
		TerminalSession: termSessID,
		ChatID:          chatID,
		LaunchSessionID: launchSessionID,
		// The selection this process was ACTUALLY launched with, recorded from the
		// same read that rendered its argv. It is the only authority on what this
		// CLI is running: nothing can ask the process later.
		LaunchModel:  sel.Model,
		LaunchEffort: sel.Effort,
		// The RESOLVED level — after resolvePermissionLevel's clamp, not the
		// chat's own raw stored intent — because this is a record of what
		// actually got launched, the same fact LaunchModel/LaunchEffort are.
		LaunchPermissionLevel: sel.PermissionLevel,
		Now:                   now,
	}); err != nil {
		return rs.teardownAfterPersistFailure(ctx, chatID, runnerID, termSessID,
			fmt.Errorf("agent: spawn runner: start runner: %w", err))
	}

	// A Start is a PLACEMENT, so it obeys the same rule a Move does: whoever else is on this
	// chat is retired. The spawn gate cannot cover this, and it is not a hairline window —
	// it is as wide as a process fork:
	//
	//	a gated SwitchProvider quits and DISPLACES the outgoing CLI, leaving the chat with
	//	nobody on it; a HOOK (never gated, and never may be) moves another live CLI onto it,
	//	evicting nobody because nobody is there; and only THEN do we resolve a descriptor,
	//	render a tmp dir, fork a process and land here.
	//
	// Without this the chat ends up holding both, indefinitely — and the loser is INVISIBLE,
	// because the serving read hands out the newest arrival while the other one goes on
	// appending to the chat's ledger. Start is SendWait, so this read already sees us.
	rs.retireOthersOn(ctx, chatID, runnerID)
	if !create {
		rs.recordChatProvider(ctx, chatID, providerID)
	}
	return nil
}

// recordChatProvider restates the chat's own durable vendor
// (domain.Chat.ProviderID) to the CLI that was just placed on it. A create seeds
// the same field through CreateInput and skips this.
//
// It runs AFTER the runner row commits: a spawn that failed placed nothing and
// must not leave a claim behind saying it did.
//
// Best-effort, like retireOthersOn above and the switch marker on switch.go's own
// path: the CLI is up and the chat is live either way, and failing a committed
// spawn over a sticky field would cost the user the session they just got. A write
// that does not land degrades to exactly the pre-field behaviour — the runner
// projections answer instead (agents.ResolveProviderID). ErrValidation is the
// ORDINARY answer here and is deliberately not logged: every respawn on the same
// provider produces one, so the event log carries a provider event only when the
// provider actually moved.
func (rs *Runners) recordChatProvider(
	ctx context.Context,
	chatID, providerID string,
) {
	if providerID == "" {
		return
	}
	if _, err := rs.chats.SetProvider(ctx, chatID, providerID); err != nil &&
		!errors.Is(err, asynxModels.ErrValidation) {
		slog.WarnContext(ctx, "agent: record the chat's own provider (best-effort, continuing)",
			"chat_id", chatID, "provider", providerID, "err", err)
	}
}

func (rs *Runners) mintRunnerToken(runnerID string) string {
	if rs.minter == nil {
		return ""
	}
	return rs.minter.Mint(runnerID)
}
