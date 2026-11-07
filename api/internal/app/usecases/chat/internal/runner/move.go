package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrChatMoving refuses a prompt while the chat's move is about to run: it would
// start a CLI in the folder the chat is leaving.
var ErrChatMoving = &problem.Error{
	Code: problem.ChatBusy, Message: "This chat is moving to its new folder; send again in a moment",
	Err: problem.ErrConflict,
}

// MoveApplier points a chat at the workspace m names and returns m as done:
// the folder left, the one entered, and what a branch-out cut.
type MoveApplier func(ctx context.Context, m ledger.Move) (ledger.Move, error)

// RequestMove records m as chatID's pending move, replacing any other, and
// reports whether it can run now.
func (rs *Runners) RequestMove(ctx context.Context, chatID string, m ledger.Move) (bool, error) {
	m.Phase = ledger.MoveRequested
	ready := false
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		var rows []ledger.Row
		if prior, ok := st.PendingMove(); ok && prior.RequestID != m.RequestID {
			gone, err := ledger.NewRow(ledger.KindMove, "", "", ledger.Move{
				Phase: ledger.MoveCancelled, RequestID: prior.RequestID, Reason: "replaced by a newer move",
			})
			if err != nil {
				return nil, err
			}
			rows = append(rows, gone)
		}
		asked, err := ledger.NewRow(ledger.KindMove, "", "", m)
		next := st.Clone()
		next.Apply(asked)
		_, ready = next.MoveReady()
		return append(rows, asked), err
	})
	if err != nil {
		return false, fmt.Errorf("agent: request move: %w", err)
	}
	rs.touch(ctx, chatID)
	return ready, nil
}

// CancelMove closes chatID's pending move, if any, for reason.
func (rs *Runners) CancelMove(ctx context.Context, chatID, reason string) error {
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return cancelPendingMove(st, reason)
	})
	if err != nil {
		return fmt.Errorf("agent: cancel move: %w", err)
	}
	rs.touch(ctx, chatID)
	return nil
}

func cancelPendingMove(st ledger.State, reason string) ([]ledger.Row, error) {
	m, ok := st.PendingMove()
	if !ok {
		return nil, nil
	}
	row, err := ledger.NewRow(ledger.KindMove, "", "", ledger.Move{
		Phase: ledger.MoveCancelled, RequestID: m.RequestID, Reason: reason,
	})
	return []ledger.Row{row}, err
}

// ExecuteMove makes chatID's pending move once nothing in the chat runs: its CLI
// ends, apply points the chat at the new workspace, and the move is recorded done
// or failed. ran is false when no move was ready.
func (rs *Runners) ExecuteMove(ctx context.Context, chatID string, apply MoveApplier) (_ ledger.Move, ran bool, _ error) {
	ctx = context.WithoutCancel(ctx)
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return ledger.Move{}, false, fmt.Errorf("agent: move: %w", err)
	}
	defer h.release()
	m, ok, err := rs.quiesceForMove(ctx, h, chatID)
	if err != nil || !ok {
		return ledger.Move{}, false, err
	}
	done, applyErr := apply(ctx, m)
	if applyErr != nil {
		done = ledger.Move{Phase: ledger.MoveFailed, RequestID: m.RequestID, Reason: problem.Of(applyErr).Message}
	} else {
		done.Phase, done.RequestID, done.Label = ledger.MoveDone, m.RequestID, m.Label
	}
	row, err := ledger.NewRow(ledger.KindMove, "", "", done)
	if err == nil {
		_, _, err = rs.ledger.Append(ctx, chatID, row)
	}
	rs.touch(ctx, chatID)
	return done, true, errors.Join(applyErr, err)
}

// quiesceForMove is the move ready to run on h's chat once its CLI has ended:
// a prompt that took the boundary while the lock was released keeps the move waiting.
func (rs *Runners) quiesceForMove(ctx context.Context, h *hold, chatID string) (ledger.Move, bool, error) {
	for {
		st, err := rs.ledger.State(ctx, chatID)
		if err != nil {
			return ledger.Move{}, false, fmt.Errorf("agent: move: %w", err)
		}
		m, ready := st.MoveReady()
		if !ready {
			return ledger.Move{}, false, nil
		}
		live, ok := rs.live.byChat(chatID)
		if !ok {
			return m, true, nil
		}
		if err := h.outside(func(context.Context) error {
			return rs.quitRunner(ctx, live, domain.AgentExitRebound)
		}); err != nil {
			return ledger.Move{}, false, fmt.Errorf("agent: move: end the CLI: %w", err)
		}
	}
}

// requireNoReadyMove refuses a prompt the chat's move must run before.
func requireNoReadyMove(st ledger.State) error {
	if _, ready := st.MoveReady(); ready {
		return ErrChatMoving
	}
	return nil
}
