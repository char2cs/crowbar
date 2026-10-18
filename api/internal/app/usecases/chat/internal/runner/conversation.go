// Package runner (file conversation.go) keeps the chat's conversation with each
// provider in the chat's ledger: what a CLI announces, and the one exit that
// loses it — the CLI refusing the session it was asked to resume.
package runner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// HandleSessionStart applies a CLI's session announcement to the chat it is
// on. The runner never moves: a session the chat does not hold is a change or
// a conflict on this chat (ledger.Announce).
func (rs *Runners) HandleSessionStart(
	ctx context.Context,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	if ev.SessionID == "" {
		return nil
	}
	changed, err := rs.announceSession(ctx, runner.CurrentChatID, runner.ProviderID, ev.SessionID)
	if err != nil {
		return err
	}
	if changed {
		rs.leaveSession(ctx, runner)
	}
	rs.reportLaunch(ctx, runner.ID, ev.Model, ev.Effort)
	return nil
}

// reportLaunch keeps what the CLI said it resolved to, for display; "" leaves
// that half as it was.
func (rs *Runners) reportLaunch(ctx context.Context, runnerID, model, effort string) {
	if model == "" && effort == "" {
		return
	}
	reported, ok := rs.live.update(runnerID, func(r *engineagents.Runner) {
		r.ReportedModel = cmp.Or(model, r.ReportedModel)
		r.ReportedEffort = cmp.Or(effort, r.ReportedEffort)
	})
	if ok {
		rs.touch(ctx, reported.CurrentChatID)
	}
}

// announceSession records that providerID's CLI on chatID is on sessionID,
// and reports whether that moved the chat off the session it was on. The
// ledger's session index alone judges a session another chat holds.
func (rs *Runners) announceSession(ctx context.Context, chatID, providerID, sessionID string) (bool, error) {
	rows, err := rs.announce(ctx, chatID, providerID, sessionID, false)
	if errors.Is(err, ledgerstore.ErrSessionClaimed) {
		rows, err = rs.announce(ctx, chatID, providerID, sessionID, true)
	}
	if err != nil {
		return false, fmt.Errorf("agent: announce session: %w", err)
	}
	for _, r := range rows {
		if m, err := ledger.DecodeBody[ledger.Marker](r); err == nil && r.Kind == ledger.KindMarker &&
			m.Kind == ledger.MarkerSessionChanged {
			return true, nil
		}
	}
	return false, nil
}

func (rs *Runners) announce(ctx context.Context, chatID, providerID, sessionID string, claimed bool) ([]ledger.Row, error) {
	rows, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Announce(st, providerID, sessionID, claimed)
	})
	return rows, err
}

// leaveSession closes what runner left open in the session it moved off: its
// turn and the prompt it carried belong to that session, so nothing on this
// chat will ever close or acknowledge them.
func (rs *Runners) leaveSession(ctx context.Context, runner engineagents.Runner) {
	chatID := runner.CurrentChatID
	rs.inflightTurns.Complete(ctx, runner.ID, domain.AgentExitMoved)
	rs.settlePrompts(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.SettleCarried(st, runner.ID, false, ledger.ReasonNotAcknowledged)
	})
	rs.salvageStreamed(ctx, chatID, runner)
	rs.abandonTurn(ctx, chatID)
}

// sessionOrigin is told the session an api connection's own driver opened or
// resumed for chatID, which binds exactly as a CLI's announcement does.
func (rs *Runners) sessionOrigin(chatID, providerID string) engineagents.APISessionOrigin {
	return func() func(string) {
		return func(sessionID string) {
			if sessionID == "" {
				return
			}
			if _, err := rs.announceSession(context.Background(), chatID, providerID, sessionID); err != nil {
				slog.Warn("agent: api transport: bind the session it opened",
					"chat_id", chatID, "provider", providerID, "err", err)
			}
		}
	}
}

// resumeWatch is what a launch that resumed a session needs at its exit to
// tell the CLI refusing that session from any other exit.
type resumeWatch struct {
	chatID, providerID, sessionID string
	agent                         engineagents.Agent
}

// loseRefusedSession loses the watched session when exit is the CLI's own
// refusal of it. It runs before the exit is reconciled, so the prompt the
// runner carried is settled as lost rather than as an ordinary exit.
func (rs *Runners) loseRefusedSession(ctx context.Context, w resumeWatch, runnerID string, exit engineterminal.Exit) {
	if w.sessionID == "" {
		return
	}
	reason, refused := w.agent.RefusedResume(exit.Code, exit.Screen)
	if refused && rs.loseSession(ctx, w.chatID, w.providerID, w.sessionID, reason, runnerID) {
		rs.live.markExiting(runnerID, domain.AgentExitResumeFailed)
	}
}

// loseAPISession loses the session an api connection's provider refused; a
// runner whose connection is its only process ends with it.
func (rs *Runners) loseAPISession(ctx context.Context, chatID string, live engineagents.Runner, sessionID string, err error) {
	if !errors.Is(err, engineagents.ErrAPISessionLost) {
		return
	}
	if rs.loseSession(ctx, chatID, live.ProviderID, sessionID, err.Error(), live.ID) {
		rs.noteRefusedRunner(live.ID)
	}
	if live.TerminalSession == "" {
		rs.apiConns.drop(live.ID)
	}
}

// loseSession moves sessionID to lost and settles what runnerID carried. It
// reports whether that lost the session.
func (rs *Runners) loseSession(ctx context.Context, chatID, providerID, sessionID, reason, runnerID string) bool {
	rows, _, err := rs.ledger.Do(context.WithoutCancel(ctx), chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Lose(st, providerID, sessionID, reason, runnerID)
	})
	if err != nil {
		slog.ErrorContext(ctx, "agent: lose a refused session", "chat_id", chatID, "err", err)
		return false
	}
	if len(rows) == 0 {
		return false
	}
	slog.InfoContext(ctx, "agent: the CLI refused the chat's session",
		"chat_id", chatID, "provider", providerID, "session_id", sessionID, "reason", reason)
	return true
}

// noteRefusedRunner notes resume_failed on a runner the refusal leaves with no
// process: its connection was all it had.
func (rs *Runners) noteRefusedRunner(runnerID string) {
	if r, ok := rs.live.byRunner(runnerID); ok && r.TerminalSession == "" {
		rs.live.markExiting(runnerID, domain.AgentExitResumeFailed)
	}
}

// boundSession is chatID's bound session with providerID, "" when there is none.
func (rs *Runners) boundSession(ctx context.Context, chatID, providerID string) (string, error) {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: bound session: %w", err)
	}
	if c, ok := st.Conversation(providerID); ok && c.State == ledger.ConversationBound {
		return c.SessionID, nil
	}
	return "", nil
}
