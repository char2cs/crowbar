package runner

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var ErrChatMoving = &problem.Error{
	Code: problem.ChatBusy, Message: "This chat is moving to its new folder; send again in a moment",
	Err: problem.ErrConflict,
}

var ErrOwnsItsBranch = problem.New(problem.Conflict,
	"This conversation already has its own branch; rename it with set_branch_name instead")

type MoveApplier func(ctx context.Context, m ledger.Move) (ledger.Move, error)

func (rs *Runners) RequestMove(ctx context.Context, chatID string, m ledger.Move) (bool, error) {
	m.Phase = ledger.MoveRequested
	ready := false
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		var rows []ledger.Row
		if prior, ok := st.PendingMove(); ok && prior.RequestID != m.RequestID {
			gone, err := ledger.NewRow(ledger.KindMove, "", "", closed(prior, ledger.MoveCancelled, "replaced by a newer move"))
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

func (rs *Runners) RequestBranchOut(ctx context.Context, chatID, workspaceID, branch, by string) (bool, error) {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: branch out: %w", err)
	}
	if m, ok := st.PendingMove(); ok && m.BranchOut {
		_, ready := st.MoveReady()
		return ready, nil
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: branch out: %w", err)
	}
	if chat.OwnsWorkspace {
		return false, ErrOwnsItsBranch
	}
	dir, _, _, err := rs.MoveSite(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return rs.RequestMove(ctx, chatID, ledger.Move{
		RequestID: uuid.NewString(), BranchOut: true, Branch: branch, From: workspaceID, FromDir: dir, By: by,
	})
}

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
	row, err := ledger.NewRow(ledger.KindMove, "", "", closed(m, ledger.MoveCancelled, reason))
	return []ledger.Row{row}, err
}

func closed(m ledger.Move, phase, reason string) ledger.Move {
	return ledger.Move{
		Phase: phase, RequestID: m.RequestID, To: m.To, ParentID: m.ParentID, Order: m.Order, Label: m.Label,
		BranchOut: m.BranchOut, Branch: m.Branch, Reason: reason,
	}
}

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
	if errors.Is(applyErr, ledger.ErrMoveWaits) {
		return ledger.Move{}, false, nil
	}
	if applyErr != nil {
		done = closed(m, ledger.MoveFailed, problem.Of(applyErr).Message)
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

func (rs *Runners) MovesWaiting(ctx context.Context) ([]string, error) {
	chats, err := rs.ledger.OpenChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: moves waiting: %w", err)
	}
	var out []string
	for _, id := range chats {
		st, err := rs.ledger.State(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("agent: moves waiting: %w", err)
		}
		if _, ready := st.MoveReady(); ready {
			out = append(out, id)
		}
	}
	return out, nil
}

func (rs *Runners) MoveSite(ctx context.Context, workspaceID string) (dir, projectID, repoID string, _ error) {
	_, projectID, repoID, dir, err := rs.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		return "", "", "", fmt.Errorf("agent: move site %s: %w", workspaceID, err)
	}
	return canonicalDir(dir), projectID, repoID, nil
}

func requireNoReadyMove(st ledger.State) error {
	if _, ready := st.MoveReady(); ready {
		return ErrChatMoving
	}
	return nil
}

func (rs *Runners) noteLevelNarrowed(
	ctx context.Context, chatID string, agent engineagents.Agent, level string, vars map[string]string,
) {
	if maps.Equal(agent.PermissionVars(level, false), vars) {
		return
	}
	rs.divide(ctx, chatID, ledger.MarkerWarning,
		"This chat now writes only inside its new folder: its permission level cannot otherwise keep it out of the one it left.")
}
