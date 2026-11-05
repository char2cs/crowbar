// Package runner (file spawnrecord.go) commits a spawned CLI to the durable
// record: the runner_started row naming the process that was just started.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// recordRunner commits forked r as the chat's runner; Launch* is what it was actually
// launched with. A process that died first has its exit recorded after, and is refused.
func (rs *Runners) recordRunner(
	ctx context.Context, r engineagents.Runner, resumeID string, staged *domain.ChatSelection,
) error {
	chatID, now := r.CurrentChatID, time.Now()
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
		return rs.teardownForked(ctx, chatID, r.ID, r.TerminalSession,
			fmt.Errorf("agent: spawn runner: record runner: %w", err))
	}
	exitedEarly := rs.live.markStarted(r.ID)
	slog.InfoContext(ctx, "agent: runner launched", "component", "runner", "chat", chatID)
	if rs.termWait != nil {
		rs.termWait.Wake() // the sweep parks while no runner is live
	}
	rs.recordChatProvider(ctx, chatID, r.ProviderID)
	rs.commitCarriedSelection(ctx, chatID, staged)
	rs.touch(ctx, chatID)
	if exitedEarly {
		return startupExit(rs.reconcileRunnerExit(context.WithoutCancel(ctx), r.ID))
	}
	return nil
}

// startupExit is what a spawn whose CLI died in startup answers, given the
// reason its exit recorded.
func startupExit(reason string) error {
	if reason == domain.AgentExitResumeFailed {
		return errRefusedInStartup{}
	}
	return ErrProviderExitedDuringStartup
}

// recordSpawnFailure frees the chat of a spawn that put no CLI on it and, for an
// existing chat, records spawn_failed, or resume_failed for a refused session;
// a CLI that died in startup, or a preempted spawn, is recorded elsewhere.
func (rs *Runners) recordSpawnFailure(ctx context.Context, chatID, runnerID string, err *error) {
	if *err == nil || errors.Is(*err, ErrProviderExitedDuringStartup) {
		return
	}
	// Claimed first, in turn order, so no hook opens a turn on it after. An exit
	// another recorder claimed leaves the zero entry, which loses nothing.
	claimed, _ := rs.claimEnd(runnerID)
	rs.live.remove(runnerID)
	// A refusal settles what the runner carried; the spawn's error names the end.
	rs.loseRefusedSession(ctx, runnerID, claimed.exit)
	if errors.Is(*err, context.Canceled) {
		return
	}
	if _, live := rs.live.byChat(chatID); live {
		return
	}
	reason := domain.AgentExitSpawnFailed
	if errors.Is(*err, engineagents.ErrAPISessionLost) {
		reason = domain.AgentExitResumeFailed
	}
	_, _, recErr := rs.ledger.Do(context.WithoutCancel(ctx), chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Exit(st, ledger.RunnerExited{RunnerID: runnerID, Reason: reason}, ledger.ReasonDeliveryFailed)
	})
	if recErr != nil {
		slog.ErrorContext(ctx, "agent: record a failed spawn", "chat_id", chatID, "runner_id", runnerID, "err", recErr)
	}
	slog.InfoContext(ctx, "agent: runner ended", "component", "runner", "chat", chatID, "reason", reason)
	rs.touch(ctx, chatID)
}

// recordChatProvider moves the chat's own durable vendor to the CLI just started
// on it, after runner_started commits, and records the switch where it moved: a
// failed spawn claims nothing. ErrValidation is a same-provider respawn.
func (rs *Runners) recordChatProvider(
	ctx context.Context,
	chatID, providerID string,
) {
	if providerID == "" {
		return
	}
	_, err := rs.chats.SetProvider(ctx, chatID, providerID)
	if errors.Is(err, asynxModels.ErrValidation) {
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "agent: record the chat's own provider (best-effort, continuing)",
			"chat_id", chatID, "provider", providerID, "err", err)
		return
	}
	rs.divide(ctx, chatID, ledger.MarkerProviderSwitched, providerID)
}

// commitCarriedSelection makes the pick a launch carried the chat's own, with
// its dividers, once its runner_started committed; nil means it carried none.
func (rs *Runners) commitCarriedSelection(ctx context.Context, chatID string, staged *domain.ChatSelection) {
	if staged == nil || rs.holdsSelection(ctx, chatID, *staged) {
		return
	}
	if err := rs.setChatSelectionLocked(ctx, chatID, staged.Model, staged.Effort); err != nil {
		slog.WarnContext(ctx, "agent: commit the carried selection (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
}

func (rs *Runners) mintRunnerToken(runnerID string) string {
	if rs.minter == nil {
		return ""
	}
	return rs.minter.Mint(runnerID)
}
