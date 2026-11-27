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

func (rs *Runners) HandleSessionStart(
	ctx context.Context,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	if ev.SessionID == "" {
		return nil
	}
	got, err := rs.announceSession(ctx, runner.CurrentChatID, runner.ProviderID, ev.SessionID, "", ev.Reason == startCleared)
	if err != nil {
		return err
	}
	switch got {
	case announcedNothing:
	case announcedChange:
		rs.leaveSession(ctx, runner)
	case announcedConflict:
		rs.endOffLock(ctx, runner, domain.AgentExitDisplaced)
		return nil
	}
	rs.reportLaunch(ctx, runner.ID, ev.Model, ev.Effort)
	return nil
}

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

type announced int

const (
	announcedNothing announced = iota
	announcedChange
	announcedConflict
)

const startCleared = "cleared"

func (rs *Runners) announceSession(ctx context.Context, chatID, providerID, sessionID, relayID string, cleared bool) (announced, error) {
	announce := func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Announce(st, providerID, sessionID, "", relayID)
	}
	if cleared {
		announce = func(st ledger.State) ([]ledger.Row, error) { return ledger.AnnounceCleared(st, providerID, sessionID) }
	}
	rows, _, err := rs.ledger.Do(ctx, chatID, announce)
	if errors.Is(err, ledgerstore.ErrSessionClaimed) {
		rows, err = rs.claimHeld(ctx, chatID, providerID, sessionID, relayID)
	}
	if err != nil {
		return announcedNothing, fmt.Errorf("agent: announce session: %w", err)
	}
	for _, r := range rows {
		m, err := ledger.DecodeBody[ledger.Marker](r)
		switch {
		case err != nil || r.Kind != ledger.KindMarker:
		case m.Kind == ledger.MarkerSessionChanged:
			return announcedChange, nil
		case m.Kind == ledger.MarkerSessionConflict:
			return announcedConflict, nil
		}
	}
	return announcedNothing, nil
}

func (rs *Runners) announce(ctx context.Context, chatID, providerID, sessionID, owner, relayID string) ([]ledger.Row, error) {
	rows, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Announce(st, providerID, sessionID, owner, relayID)
	})
	return rows, err
}

func (rs *Runners) leaveSession(ctx context.Context, runner engineagents.Runner) {
	chatID := runner.CurrentChatID
	command := rs.commandOf(ctx, runner)
	rs.settlePrompts(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		salvaged, err := rs.turns.ExitRows(st, chatID, runner.ID)
		if err != nil {
			return nil, err
		}
		left, err := ledger.Leave(st, runner.ID, command)
		return append(salvaged, left...), err
	})
	rs.turns.Retire(chatID, runner.ID)
}

func (rs *Runners) commandOf(ctx context.Context, runner engineagents.Runner) func(string) bool {
	crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return nil
	}
	agent, err := rs.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return nil
	}
	return agent.PromptIsCommand
}

func (rs *Runners) sessionOrigin(chatID, providerID, relayID string) engineagents.APISessionOrigin {
	return func() func(string) {
		return func(sessionID string) {
			if sessionID == "" {
				return
			}
			if _, err := rs.announceSession(context.Background(), chatID, providerID, sessionID, relayID, false); err != nil {
				slog.Warn("agent: api transport: bind the session it opened",
					"chat_id", chatID, "provider", providerID, "err", err)
			}
		}
	}
}

type resumeWatch struct {
	chatID, providerID, sessionID string
	agent                         engineagents.Agent
}

type runnerExit struct {
	engineterminal.Exit
	resume resumeWatch
}

func (rs *Runners) loseRefusedSession(ctx context.Context, runnerID string, exit runnerExit) string {
	w := exit.resume
	if w.sessionID == "" {
		return ""
	}
	reason, refused := w.agent.RefusedResume(exit.Code, exit.Screen)
	if !refused || !rs.loseSession(ctx, w.chatID, w.providerID, w.sessionID, reason, runnerID) {
		return ""
	}
	return domain.AgentExitResumeFailed
}

func (rs *Runners) loseAPISession(ctx context.Context, chatID string, live engineagents.Runner, sessionID string, err error) {
	if !errors.Is(err, engineagents.ErrAPISessionLost) {
		return
	}
	if rs.loseSession(ctx, chatID, live.ProviderID, sessionID, err.Error(), live.ID) {
		rs.noteRefusedRunner(chatID, live.ID)
	}
	if live.TerminalSession == "" {
		rs.apiConns.drop(live.ID)
	}
}

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

func (rs *Runners) noteRefusedRunner(chatID, runnerID string) {
	if v, ok := rs.live.view(chatID); ok && v.runner.ID == runnerID && v.started && v.runner.TerminalSession == "" {
		rs.live.noteCause(runnerID, domain.AgentExitResumeFailed)
	}
}

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
