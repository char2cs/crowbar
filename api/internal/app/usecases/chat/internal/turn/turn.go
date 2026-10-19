package turn

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (t *Turns) openAssistantTurn(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
) {
	t.openAssistantTurnWithPlan(ctx, chat, runner, false)
}

func (t *Turns) openAssistantTurnWithPlan(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	carryPlan bool,
) {
	if err := t.activity.OpenTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     openTurnID(chat.ID, runner.ID),
		ProviderID: runner.ProviderID,
		RunnerID:   runner.ID,
		SessionID:  t.boundSession(ctx, chat.ID, runner.ProviderID),
		CarryPlan:  carryPlan,
		Now:        time.Now(),
	}); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: open assistant turn",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	}
}

func openTurnID(chatID, runnerID string) string {
	return "open-" + chatID + "-" + runnerID
}

func (t *Turns) handleTurn(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	chat, ok, err := t.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}

	switch ev.Kind {
	case "user_prompt":
		_, err := t.openingTurn(runner.ID, func(h turnHold) error {
			return t.openTurnFromPrompt(ctx, chat, h, agent, ev)
		})
		return err
	case "turn_stop":
		return t.closeTurnFromStop(ctx, chat, runner, agent, ev)
	case engineagents.HookTurnFailed:
		return t.closeTurnFromFailure(ctx, chat, runner, ev)
	}
	return nil
}

// runningOn is the turn ev ends: the chat's running turn, when runnerID runs
// it and ev names it. Any other end is late.
func runningOn(st ledger.State, runnerID string, ev engineagents.CanonicalEvent) bool {
	turn, ok := st.RunningTurn()
	return ok && turn.RunnerID == runnerID && turn.Names(ev.TurnID)
}

func (t *Turns) closeTurnFromStop(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	// A compaction's own turn envelope closing is not an assistant reply.
	if t.compacting.consume(chat.ID, ev.TurnID) {
		return nil
	}
	// The answer is recorded before anyone is told the turn ended: a client
	// reads the record once on that edge.
	appendErr := t.closeAssistantTurn(ctx, chat, runner, ev)
	var steered *ledger.Prompt
	var reply string
	steerTurn := t.turnStarted(ctx, chat.ID, runner, agent, "")
	end := func(held bool) error {
		_, _, err := t.ledger.Do(ctx, chat.ID, func(st ledger.State) ([]ledger.Row, error) {
			steered = nil
			if !runningOn(st, runner.ID, ev) {
				return nil, nil
			}
			rows, err := ledger.EndTurn(st, ledger.TurnStatusDone, "", ev.AsyncWork, ev.Effort)
			p, ok := pendingSteer(st, runner.ID)
			if err != nil || !ok {
				return rows, err
			}
			stdout, deliverable := t.steerReply(ctx, chat.ID, p, agent)
			if !held || !deliverable {
				settled, err := settleSteer(st, runner.ID, ledger.ReasonDeliveryFailed)
				return append(rows, settled...), err
			}
			steerTurn.RequestID = p.RequestID
			started, err := ledger.NewRow(ledger.KindTurnStarted, p.TurnID, "", steerTurn)
			steered, reply = &p, stdout
			return append(rows, started), err
		})
		return err
	}
	// Held against the runner's end, so the steered prompt's turn never opens
	// on a CLI that is going away.
	opened, err := t.openingTurn(runner.ID, func(turnHold) error { return end(true) })
	if !opened {
		err = end(false)
	}
	if err != nil {
		return fmt.Errorf("agent: ingest hook: end turn: %w", err)
	}
	if steered != nil {
		t.replies.put(inflight.DeliveryID(ctx), reply)
		t.recordSteered(ctx, chat, runner, *steered)
	}
	return appendErr
}

func (t *Turns) closeTurnFromFailure(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	if t.compacting.consume(chat.ID, ev.TurnID) {
		return nil
	}
	appendErr := t.closeAssistantTurn(ctx, chat, runner, ev)
	if reason := failureNotice(ev); reason != "" {
		note(ctx, "record turn failure", t.conversations.RecordTurn(
			ctx, chat, runner.ProviderID, runner.ID, t.boundSession(ctx, chat.ID, runner.ProviderID),
			domain.TurnRoleNotice, reason, "",
		))
	}
	reason, detail := ledger.ReasonTurnFailed, ""
	if ev.Failure != nil {
		reason, detail = cmp.Or(ev.Failure.Reason, reason), ev.Failure.Detail
	}
	if err := t.failTurn(ctx, chat.ID, runner.ID, func(st ledger.State) bool {
		return runningOn(st, runner.ID, ev)
	}, reason, detail); err != nil {
		return fmt.Errorf("agent: ingest hook: fail turn: %w", err)
	}
	return appendErr
}

// failTurn records the running turn failed when ends says this failure is
// its, settling the steered prompt it would have delivered.
func (t *Turns) failTurn(
	ctx context.Context, chatID, runnerID string, ends func(ledger.State) bool, reason, detail string,
) error {
	_, _, err := t.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		if !ends(st) {
			return nil, nil
		}
		rows, err := ledger.FailTurn(st, reason, detail)
		if err != nil {
			return nil, err
		}
		settled, err := settleSteer(st, runnerID, ledger.ReasonTurnFailed)
		return append(rows, settled...), err
	})
	return err
}

func failureNotice(ev engineagents.CanonicalEvent) string {
	if ev.Failure == nil || ev.Failure.Reason == "" {
		return ""
	}
	if ev.Failure.Detail != "" {
		return ev.Failure.Reason + ": " + ev.Failure.Detail
	}
	return ev.Failure.Reason
}

// endIdleTurn ends turn on its own runner's idle report, and the steer it
// carried with it.
func endIdleTurn(st ledger.State, turn ledger.Turn, runnerID string) ([]ledger.Row, error) {
	if turn.RunnerID != runnerID {
		return nil, nil
	}
	rows, err := ledger.EndTurn(st, ledger.TurnStatusDone, ledger.ReasonProviderIdle, 0, "")
	if err != nil {
		return nil, err
	}
	settled, err := settleSteer(st, runnerID, ledger.ReasonDeliveryFailed)
	return append(rows, settled...), err
}

// recordIdle is the provider saying it is doing nothing: the turn runner was
// running is over. With no turn running, a hooks CLI's report means the
// prompts it carried were never taken; an api connection's says nothing.
func (t *Turns) recordIdle(ctx context.Context, chatID string, runner engineagents.Runner) {
	api := inflight.FromAPITransport(ctx)
	_, _, err := t.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		if turn, ok := st.RunningTurn(); ok {
			return endIdleTurn(st, turn, runner.ID)
		}
		if api {
			return nil, nil
		}
		return ledger.Settle(st, func(p ledger.Prompt) bool {
			return p.RunnerID == runner.ID && p.Delivery != ledger.DeliverySteer
		}, false, ledger.ReasonNotAcknowledged)
	})
	note(ctx, "provider idle", err)
}

// RecordStop records the divider for a turn a person stopped. It holds
// runnerID's hook gate, so it never lands inside a hook's ingest.
func (t *Turns) RecordStop(ctx context.Context, chatID, runnerID string) error {
	defer t.hookGates.Lock(runnerID)()
	now := time.Now()
	id := "interrupt-" + fallbackID()
	if err := t.activity.Interrupt(
		ctx, chatID, id, engineagents.InterruptStopped, "", now,
	); err != nil {
		return fmt.Errorf("agent: record stop: interrupt: %w", err)
	}
	if err := t.activity.ResolveInterruption(
		ctx, chatID, id, engineagents.InterruptStopped, "", now,
	); err != nil {
		return fmt.Errorf("agent: record stop: resolve interruption: %w", err)
	}
	return nil
}

// RecordChatSwitch notes, durably, that Crowbar itself changed chatID's
// provider, model or effort — Crowbar's own doing, the same as RecordStop,
// never something a provider hook reports. kind is one of
// InterruptProviderSwitched/InterruptModelChanged/InterruptEffortChanged;
// detail is the new value. Opened and resolved in the same call, back to
// back, exactly like RecordStop: Crowbar already knows the full story the
// instant it decides to make the change, so there is no later event to
// close it on. The caller decides whether the value actually changed —
// this method has no "old" to compare against, so it always records.
func (t *Turns) RecordChatSwitch(ctx context.Context, chatID, kind, detail string) error {
	now := time.Now()
	id := "interrupt-" + fallbackID()
	if err := t.activity.Interrupt(ctx, chatID, id, kind, detail, now); err != nil {
		return fmt.Errorf("agent: record chat switch: interrupt: %w", err)
	}
	if err := t.activity.ResolveInterruption(ctx, chatID, id, kind, detail, now); err != nil {
		return fmt.Errorf("agent: record chat switch: resolve interruption: %w", err)
	}
	return nil
}

func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}
