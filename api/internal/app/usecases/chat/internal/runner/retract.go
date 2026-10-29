package runner

import (
	"context"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// What a retract did to its prompt.
const (
	RetractRetracted   = "retracted"   // withdrawn before the provider took it
	RetractInterrupted = "interrupted" // its running turn was ended
)

// RetractResult is a retract's outcome and the prompt's text, for the composer.
type RetractResult struct {
	State string
	Text  string
}

// retraction is what a retract decided under the chat's writer: its rows, and
// the CLI it ends (carrier) under cause, if any.
type retraction struct {
	result  RetractResult
	rows    []ledger.Row
	carrier string
	cause   string
}

// Retract withdraws requestID: pending, it settles and the CLI launched to
// carry it is retired; delivered, its running turn ends and the provider is
// signalled as Stop does. The row commits before any signal, which never waits.
func (rs *Runners) Retract(ctx context.Context, chatID, requestID string) (RetractResult, error) {
	unlock, err := rs.chatLock.Lock(ctx, chatID)
	if err != nil {
		return RetractResult{}, fmt.Errorf("agent: retract: %w", err)
	}
	var r retraction
	_, _, err = rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		var err error
		r, err = retract(st, requestID)
		return r.rows, err
	})
	retiring := false
	v, live := rs.live.view(chatID)
	if err == nil && live && r.carrier != "" && v.runner.ID == r.carrier {
		retiring = rs.signal(ctx, v.runner, r)
	}
	unlock()
	if err != nil {
		return RetractResult{}, fmt.Errorf("agent: retract: %w", err)
	}
	if !retiring {
		return r.result, nil
	}
	// Outside the lock, as Stop: the CLI's own hooks on the way down need it.
	if err := rs.endMarked(ctx, v.runner); err != nil {
		return RetractResult{}, fmt.Errorf("agent: retract: %w", err)
	}
	return r.result, nil
}

// signal tells carrier's provider, under the chat's lock as Stop does, so no
// send decides against it meanwhile: an interrupted turn ends in place where
// it can; otherwise the CLI is marked ending, and true is its caller's to end.
func (rs *Runners) signal(ctx context.Context, carrier agents.Runner, r retraction) bool {
	if r.result.State == RetractInterrupted && rs.interruptTurn(ctx, carrier) {
		return false
	}
	_, retiring := rs.markEnding(carrier, r.cause)
	return retiring
}

// retract decides requestID's withdrawal from st.
func retract(st ledger.State, requestID string) (retraction, error) {
	p, ok := st.Receipt(requestID)
	if !ok {
		return retraction{}, fmt.Errorf("prompt %s: %w", requestID, problem.ErrNotFound)
	}
	if p.Status == ledger.PromptStatusPending {
		rows, err := ledger.Settle(st, func(q ledger.Prompt) bool { return q.RequestID == requestID },
			false, ledger.ReasonRetracted)
		r := retraction{result: RetractResult{State: RetractRetracted, Text: p.Text}, rows: rows}
		if p.Delivery == ledger.DeliveryRestart {
			r.carrier, r.cause = p.RunnerID, domain.AgentExitRetracted
		}
		return r, err
	}
	turn, running := st.RunningTurn()
	if p.Status != ledger.PromptStatusDelivered || !running || turn.RequestID != requestID {
		return retraction{}, fmt.Errorf("prompt %s is settled or its turn closed: %w", requestID, problem.ErrConflict)
	}
	rows, err := ledger.Stopping(st)
	if err != nil {
		return retraction{}, err
	}
	// Only this turn's subagents: one an earlier turn left running is not its to stop.
	rows = slices.DeleteFunc(rows, func(row ledger.Row) bool { return row.TurnID != turn.ID })
	ended, err := ledger.EndTurn(st, ledger.TurnStatusInterrupted, ledger.ReasonRetracted)
	return retraction{
		result: RetractResult{State: RetractInterrupted, Text: p.Text}, rows: append(rows, ended...),
		carrier: turn.RunnerID, cause: domain.AgentExitStopped,
	}, err
}
