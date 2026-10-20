package turn

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
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

// handleTurn records a turn's start or end. A turn's end may owe its sender
// the stdout that carries a steered prompt into the same process.
func (t *Turns) handleTurn(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) ([]byte, error) {
	chat, err := t.chatForRunner(ctx, runner)
	if err != nil {
		return nil, err
	}

	switch ev.Kind {
	case engineagents.HookUserPrompt:
		_, err := t.openingTurn(runner.ID, func(h turnHold) error {
			return t.openTurnFromPrompt(ctx, chat, h, agent, ev)
		})
		return nil, err
	case engineagents.HookTurnStop:
		return t.closeTurnFromStop(ctx, chat, runner, agent, ev)
	case engineagents.HookTurnFailed:
		return nil, t.closeTurnFromFailure(ctx, chat, runner, ev)
	}
	return nil, nil
}

// stopStatus is how a turn_stop's provider status ended its turn: anything but
// a completed turn did not finish its work.
func stopStatus(providerStatus string) ledger.TurnStatus {
	if providerStatus == "" || providerStatus == "completed" {
		return ledger.TurnStatusDone
	}
	return ledger.TurnStatusInterrupted
}

func (t *Turns) closeTurnFromStop(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) ([]byte, error) {
	// A compaction's own turn envelope closing is not an assistant reply.
	if t.compacting.consume(chat.ID, ev.TurnID) {
		return nil, nil
	}
	// The answer is recorded before anyone is told the turn ended: a client
	// reads the record once on that edge.
	appendErr := t.closeAssistantTurn(ctx, chat, runner, ev)
	var steered *ledger.Prompt
	var reply []byte
	steerTurn := t.turnStarted(ctx, chat.ID, runner, agent, "")
	// A reply that was not recorded refuses this hook, and the steer with it.
	end := func(deliver bool) error {
		_, _, err := t.ledger.Do(ctx, chat.ID, func(st ledger.State) ([]ledger.Row, error) {
			steered = nil
			ended, closes, err := ledger.EndTurnOf(st, runner.ID, ev.TurnID, ledger.TurnEnded{
				Status: string(stopStatus(ev.TurnStatus)), AsyncWork: &ev.AsyncWork, Effort: ev.Effort,
			})
			rows := []ledger.Row{ended}
			p, ok := pendingSteer(st, runner.ID)
			if err != nil || !closes || !ok {
				return rows, err
			}
			stdout, deliverable := steerReply(ctx, p, agent)
			if !deliver || !deliverable || !startsOn(st, runner.ID) {
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
	opened, err := t.openingTurn(runner.ID, func(turnHold) error { return end(appendErr == nil) })
	if !opened {
		err = end(false)
	}
	if err != nil {
		return nil, fmt.Errorf("agent: ingest hook: end turn: %w", err)
	}
	if steered == nil {
		return nil, appendErr
	}
	t.recordSteered(ctx, chat, runner, *steered)
	return reply, appendErr
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
		appendErr = errors.Join(appendErr, wrote("record turn failure", t.conversations.RecordTurn(
			ctx, chat, "", runner.ProviderID, runner.ID, t.boundSession(ctx, chat.ID, runner.ProviderID),
			domain.TurnRoleNotice, reason, "",
		)))
	}
	reason, detail := ledger.ReasonTurnFailed, ""
	if ev.Failure != nil {
		reason, detail = cmp.Or(ev.Failure.Reason, reason), ev.Failure.Detail
	}
	_, _, err := t.ledger.Do(ctx, chat.ID, func(st ledger.State) ([]ledger.Row, error) {
		rows, _, err := failRows(st, runner.ID, ev.TurnID, reason, detail)
		return rows, err
	})
	if err != nil {
		return fmt.Errorf("agent: ingest hook: fail turn: %w", err)
	}
	return appendErr
}

// failRows is runnerID's turn_failed row for the turn providerTurnID names,
// late or not, and whether it closes the running turn and the steer it carried.
func failRows(st ledger.State, runnerID, providerTurnID, reason, detail string) ([]ledger.Row, bool, error) {
	failed, closes, err := ledger.FailTurnOf(st, runnerID, providerTurnID, ledger.TurnFailed{Reason: reason, Detail: detail})
	rows := []ledger.Row{failed}
	if err != nil || !closes {
		return rows, closes, err
	}
	settled, err := settleSteer(st, runnerID, ledger.ReasonTurnFailed)
	return append(rows, settled...), true, err
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

// recordIdle is the provider saying it is idle: runner's running turn is over,
// its steer undelivered, any other end late. With no turn, a hooks CLI's report
// means the prompts it carried were never taken; an api connection's, nothing.
func (t *Turns) recordIdle(ctx context.Context, chatID string, runner engineagents.Runner) error {
	api := hookChannel(ctx) == engineagents.ChannelAPI
	_, _, err := t.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		ended, closes, err := ledger.EndTurnOf(st, runner.ID, "", ledger.TurnEnded{
			Status: string(ledger.TurnStatusDone), Reason: ledger.ReasonProviderIdle,
		})
		rows := []ledger.Row{ended}
		if err != nil {
			return nil, err
		}
		if closes {
			settled, err := settleSteer(st, runner.ID, ledger.ReasonDeliveryFailed)
			return append(rows, settled...), err
		}
		if _, running := st.RunningTurn(); running || api {
			return rows, nil
		}
		settled, err := ledger.Settle(st, func(p ledger.Prompt) bool {
			return p.RunnerID == runner.ID && p.Delivery != ledger.DeliverySteer
		}, false, ledger.ReasonNotAcknowledged)
		return append(rows, settled...), err
	})
	return wrote("provider idle", err)
}

// RecordStop records the divider for a turn a person stopped. The caller holds
// the chat's lock, so it never lands inside a hook's ingest.
func (t *Turns) RecordStop(ctx context.Context, chatID string) error {
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
