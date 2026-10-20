// Package runner (file spawnrecord.go) commits a spawned CLI to the durable
// record: the chat row a create has to mint first, then the runner_started row
// naming the process that was just started.
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
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// recordRunner commits r, already forked and in the live table, as the chat's
// runner. Launch* is what the process was actually launched with: nothing can
// ask it later. A process that died before this row committed has its exit
// recorded after it, and the spawn refused.
func (rs *Runners) recordRunner(ctx context.Context, r engineagents.Runner, resumeID string, create bool) error {
	chatID, now := r.CurrentChatID, time.Now()
	if create {
		_, err := rs.chats.Create(ctx, agentchat.CreateInput{
			ID:          chatID,
			WorkspaceID: r.WorkspaceID,
			Type:        domain.ChatTypeChat,
			ProviderID:  r.ProviderID,
			Now:         now,
		})
		if err != nil {
			return rs.teardownAfterPersistFailure(ctx, chatID, r.ID, r.TerminalSession,
				fmt.Errorf("agent: spawn runner: create chat: %w", err))
		}
		rs.seedPermissionLevel(ctx, chatID)
	}
	rs.live.update(r.ID, func(live *engineagents.Runner) {
		live.TerminalSession = r.TerminalSession
		live.LaunchModel, live.LaunchEffort = r.LaunchModel, r.LaunchEffort
		live.LaunchPermissionLevel = r.LaunchPermissionLevel
		live.StartedAt = now
	})
	row, err := ledger.NewRow(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{
		RunnerID: r.ID, Provider: r.ProviderID, TerminalSession: r.TerminalSession,
		LaunchModel: r.LaunchModel, LaunchEffort: r.LaunchEffort,
		LaunchPermissionLevel: r.LaunchPermissionLevel, ResumeSessionID: resumeID,
	})
	if err == nil {
		_, _, err = rs.ledger.Append(ctx, chatID, row)
	}
	if err != nil {
		return rs.teardownAfterPersistFailure(ctx, chatID, r.ID, r.TerminalSession,
			fmt.Errorf("agent: spawn runner: record runner: %w", err))
	}
	exitedEarly := rs.live.markStarted(r.ID)
	slog.InfoContext(ctx, "agent: runner launched", "component", "runner", "chat", chatID)
	if rs.termWait != nil {
		rs.termWait.Wake() // the sweep parks while no runner is live
	}
	if !create {
		rs.recordChatProvider(ctx, chatID, r.ProviderID)
	}
	rs.announceRunner(ctx, chatID, r.ID, frameStarted)
	if exitedEarly {
		rs.reconcileRunnerExit(context.WithoutCancel(ctx), r.ID)
		return ErrProviderExitedDuringStartup
	}
	return nil
}

// recordSpawnFailure frees the chat of a spawn that put no CLI on it and, for an
// existing chat, records spawn_failed. A CLI that died during startup recorded
// its own exit; a preempted spawn is the preempting Stop's to record.
func (rs *Runners) recordSpawnFailure(ctx context.Context, chatID, runnerID string, create bool, err *error) {
	if *err == nil || errors.Is(*err, ErrProviderExitedDuringStartup) {
		return
	}
	// Claimed first, in turn order, so no hook opens a turn on it after.
	rs.claimEnd(runnerID)
	rs.live.remove(runnerID)
	if create || errors.Is(*err, context.Canceled) {
		return
	}
	if _, live := rs.live.byChat(chatID); live {
		return
	}
	_, _, recErr := rs.ledger.Do(context.WithoutCancel(ctx), chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Exit(st, ledger.RunnerExited{RunnerID: runnerID, Reason: domain.AgentExitSpawnFailed},
			ledger.ReasonDeliveryFailed)
	})
	if recErr != nil {
		slog.ErrorContext(ctx, "agent: record a failed spawn", "chat_id", chatID, "runner_id", runnerID, "err", recErr)
	}
	slog.InfoContext(ctx, "agent: runner ended", "component", "runner", "chat", chatID,
		"reason", domain.AgentExitSpawnFailed)
	rs.announceRunner(ctx, chatID, runnerID, frameExited)
}

// recordChatProvider restates the chat's own durable vendor to the CLI just
// started on it, after runner_started commits: a failed spawn claims nothing.
// Best-effort; ErrValidation is the ordinary answer of a same-provider respawn.
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
