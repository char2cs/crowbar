package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (u *turnUsecase) IngestHook(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	// Check the startup barrier BEFORE the repository. Once recordRunner commits,
	// the row is visible, but the barrier deliberately remains installed through
	// ordered replay; consulting only the repository here would let a later hook
	// overtake the buffered session_start/user_prompt batch.
	if handled, err := u.pendingHooks.enqueue(runnerID, provider, canonicalEvent, rawPayload); handled {
		return err
	}
	return u.ingestHookNow(ctx, runnerID, provider, canonicalEvent, rawPayload)
}

func (u *turnUsecase) ingestUserPromptInterlocked(
	ctx context.Context,
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	for {
		runner, err := u.runners.Get(ctx, runnerID)
		if err != nil {
			if errors.Is(err, agentrunner.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("agent: ingest hook: runner: %w", err)
		}
		if runner.CurrentChatID == "" {
			return nil
		}
		chatID := runner.CurrentChatID
		unlock := u.turnStarts.lock(chatID)
		current, err := u.runners.Get(ctx, runnerID)
		if err != nil {
			unlock()
			if errors.Is(err, agentrunner.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("agent: ingest hook: refresh runner under turn-start interlock: %w", err)
		}
		if current.CurrentChatID != chatID {
			unlock()
			// Placement changed before the lock. Retry against the durable current
			// chat, or drop on the next iteration if the runner was displaced.
			continue
		}
		err = u.ingestResolvedHook(ctx, current, provider, canonicalEvent, rawPayload)
		unlock()
		return err
	}
}

func (u *turnUsecase) ingestResolvedHook(
	ctx context.Context,
	runner engineagents.Runner,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	runnerID := runner.ID

	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: worktree dir: %w", err)
	}

	// The runner is the source of truth for which provider spawned this CLI. The
	// hook's self-reported provider is only a guard against a mis-authored descriptor.
	if provider != "" && provider != runner.ProviderID {
		slog.WarnContext(ctx, "agent: ingest hook: provider mismatch",
			"hook_provider", provider, "runner_provider", runner.ProviderID, "runner_id", runnerID)
	}

	descriptor, err := u.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	// Telemetry arrives over the same relay as a hook — a command, JSON on stdin,
	// scoped to the same segment — but it is not a conversation event. It carries
	// no session, describes no turn, and must not be run through the ownership
	// guard, which asks a question about conversations.
	if canonicalEvent == engineagents.HookTelemetry {
		return u.handleTelemetry(ctx, runner, descriptor, rawPayload)
	}

	// One call, deliberately: decode, ownership-check and map are fused in the
	// engine so the middle step cannot be skipped. It is the guard that stops a
	// provider's own internal session being filed as the user's conversation, and
	// a guard a caller can forget to call is one that will eventually be forgotten.
	//
	// Every failure here DROPS the hook rather than failing it. A hook must never
	// break the vendor CLI's turn.
	ev, err := descriptor.ParseHook(canonicalEvent, rawPayload)
	if err != nil {
		switch {
		case errors.Is(err, engineagents.ErrForeignConversation):
			slog.DebugContext(ctx, "agent: ingest hook: dropping a hook that is not this CLI's own conversation",
				"reason", err, "event", canonicalEvent,
				"provider", runner.ProviderID, "runner_id", runnerID)
		case errors.Is(err, engineagents.ErrHookUndeclared):
			slog.DebugContext(ctx, "agent: ingest hook: provider does not map this event",
				"event", canonicalEvent, "provider", runner.ProviderID, "runner_id", runnerID)
		default:
			slog.WarnContext(ctx, "agent: ingest hook: parse payload",
				"err", err, "event", canonicalEvent, "runner_id", runnerID)
		}
		return nil
	}

	// Where does this conversation's own transcript stand RIGHT NOW? Asked on every
	// hook and answered once per file: a session Crowbar has not been watching must
	// start at the file's end, or a resumed conversation's whole history would be
	// replayed into the record as though the agent had just said all of it. Asked
	// here rather than on the announcement alone because a daemon that restarts
	// mid-session never sees that session's announcement.

	switch ev.Kind {
	case engineagents.HookSessionStart:
		return u.runner.handleSessionStart(ctx, runner, ev)
	case engineagents.HookUserPrompt, engineagents.HookTurnStop, engineagents.HookTurnFailed:
		return u.handleTurn(ctx, runner, descriptor, ev)
	case engineagents.HookMessageDelta,
		engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail,
		engineagents.HookSubagentPre, engineagents.HookSubagentPost,
		engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation,
		engineagents.HookCompactPre, engineagents.HookCompactPost:
		return u.handleObservation(ctx, runner, descriptor, ev, rawPayload)
	case engineagents.HookSessionEnd:
		// A session ending is already observed authoritatively by the PTY exit
		// reconcile, which runs whether or not the CLI got to fire a hook. Acting
		// on it here as well would close a turn twice.
		return nil
	}
	return nil
}

func (u *turnUsecase) replayStartupHook(
	runnerID string,
	hook pendingRunnerHook,
) {
	replayCtx := context.Background()
	if hook.deliveryID != "" {
		replayCtx = context.WithValue(replayCtx, hookDeliveryContextKey{}, hook.deliveryID)
	}
	if err := u.ingestHookNow(
		replayCtx, runnerID, hook.provider, hook.canonicalEvent, hook.rawPayload,
	); err != nil {
		slog.Error("agent: replay startup hook (best-effort, continuing)",
			"runner_id", runnerID, "event", hook.canonicalEvent, "err", err)
		return
	}
	if hook.deliveryID == "" {
		return
	}
	if err := u.hookDeliveries.Complete(
		hook.deliveryDir, hook.deliveryID, hook.deliveryHash, time.Now(),
	); err != nil {
		slog.Error("agent: persist replayed startup hook delivery (effects already committed)",
			"runner_id", runnerID, "delivery_id", hook.deliveryID, "err", err)
	}
}

func (u *turnUsecase) ingestHookNow(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	if canonicalEvent == "user_prompt" {
		return u.ingestUserPromptInterlocked(ctx, runnerID, provider, canonicalEvent, rawPayload)
	}
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		if errors.Is(err, agentrunner.ErrNotFound) {
			// A hook from a runner we do not know (or one whose PTY has already died):
			// ignore it. Never resurrect a runner from a hook.
			return nil
		}
		return fmt.Errorf("agent: ingest hook: runner: %w", err)
	}
	return u.ingestResolvedHook(ctx, runner, provider, canonicalEvent, rawPayload)
}

func (u *turnUsecase) chatForRunner(
	ctx context.Context,
	runner engineagents.Runner,
) (domain.Chat, bool, error) {
	if runner.CurrentChatID == "" {
		// The runner is placed NOWHERE: Crowbar has taken it off its chat and is killing
		// it (a switch, an eviction, a chat deleted under it), and a SIGTERM'd CLI keeps
		// talking for a moment. Its turns belong to nobody, and nowhere is never looked up
		// — GetChat("") would miss and trigger agentchat's lazy self-heal, replaying the
		// ENTIRE event log, on every hook of a dying CLI.
		return domain.Chat{}, false, nil
	}
	chat, err := u.chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			// The chat was deleted out from under the CLI (which is still dying). A turn
			// typed into a chat the user has just removed goes nowhere, by design.
			return domain.Chat{}, false, nil
		}
		return domain.Chat{}, false, fmt.Errorf("agent: ingest hook: chat: %w", err)
	}
	return chat, true, nil
}
