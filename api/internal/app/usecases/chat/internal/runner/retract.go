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

const (
	RetractRetracted   = "retracted"
	RetractInterrupted = "interrupted"
)

type RetractResult struct {
	State string
	Text  string
}

type retraction struct {
	result  RetractResult
	rows    []ledger.Row
	carrier string
	cause   string
}

func (rs *Runners) Retract(ctx context.Context, chatID, requestID string) (RetractResult, error) {
	unlock, err := rs.chatLock.Lock(ctx, chatID)
	if err != nil {
		return RetractResult{}, fmt.Errorf("agent: retract: %w", err)
	}
	v, live := rs.live.view(chatID)
	held := func(p ledger.Prompt) bool {
		return live && rs.cliHolds(v.runner.ID, p)
	}
	var r retraction
	_, _, err = rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		var err error
		r, err = retract(st, requestID, held)
		return r.rows, err
	})
	retiring := false
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
	if err := rs.endMarked(ctx, v.runner); err != nil {
		return RetractResult{}, fmt.Errorf("agent: retract: %w", err)
	}
	return r.result, nil
}

func (rs *Runners) signal(ctx context.Context, carrier agents.Runner, r retraction) bool {
	if r.result.State == RetractInterrupted && rs.interruptTurn(ctx, carrier) {
		return false
	}
	_, retiring := rs.markEnding(carrier, r.cause)
	return retiring
}

func (rs *Runners) cliHolds(runnerID string, p ledger.Prompt) bool {
	if p.RunnerID != runnerID {
		return false
	}
	switch p.Delivery {
	case ledger.DeliveryAPI, ledger.DeliveryType:
		return true
	case ledger.DeliveryRestart:
		return rs.HasLiveAPIConnection(runnerID)
	}
	return false
}

func retract(st ledger.State, requestID string, held func(ledger.Prompt) bool) (retraction, error) {
	p, ok := st.Receipt(requestID)
	if !ok {
		return retraction{}, fmt.Errorf("prompt %s: %w", requestID, problem.ErrNotFound)
	}
	if p.Status == ledger.PromptStatusPending && held(p) {
		return retraction{}, ErrPromptWithProvider
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
	rows = slices.DeleteFunc(rows, func(row ledger.Row) bool { return row.TurnID != turn.ID })
	ended, err := ledger.EndTurn(st, ledger.TurnStatusInterrupted, ledger.ReasonRetracted)
	return retraction{
		result: RetractResult{State: RetractInterrupted, Text: p.Text}, rows: append(rows, ended...),
		carrier: turn.RunnerID, cause: domain.AgentExitStopped,
	}, err
}
