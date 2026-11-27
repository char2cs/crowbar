package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (rs *Runners) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
	surface string,
) (string, error) {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: %w", err)
	}
	defer h.release()
	defer rs.enterPhase(ctx, chatID, rs.replacementPhase(chatID))()
	if surface != "" {
		if err := rs.moveDormantSurface(ctx, chatID, surface); err != nil {
			return "", err
		}
	}
	return rs.switchProviderLocked(ctx, h, chatID, targetProviderID)
}

func (rs *Runners) switchProviderLocked(
	ctx context.Context,
	h *hold,
	chatID string,
	targetProviderID string,
) (string, error) {
	if err := rs.providers.RequireProviderEnabled(ctx, targetProviderID); err != nil {
		return "", err
	}
	for {
		if _, _, err := rs.settledRunner(h); err != nil {
			return "", fmt.Errorf("agent: switch provider: %w", err)
		}
		chat, err := rs.chats.GetChat(ctx, chatID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: chat: %w", err)
		}
		if err := rs.settleStaleWork(ctx, chatID); err != nil {
			return "", err
		}
		if err := rs.requireNotBusy(ctx, chatID); err != nil {
			return "", err
		}
		waited, err := rs.waitOutPendingPrompt(ctx, h, chatID)
		if err != nil {
			return "", err
		}
		if waited {
			continue
		}
		cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chatID, chat.WorkspaceID)
		if err != nil {
			return "", err
		}
		crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: preflight worktree dir: %w", err)
		}
		d, err := rs.agents.Get(ctx, crowbarHome, targetProviderID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: resolve descriptor: %w", err)
		}
		launch, err := rs.planLaunch(ctx, chatID, d)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: %w", err)
		}
		resuming := launch.resumeID != ""
		leftAt, err := rs.leftAt(ctx, chatID, targetProviderID, launch.resumeID)
		if err != nil {
			return "", err
		}

		conversation, err := rs.conversations.AssembleConversation(ctx, chatID, resuming, leftAt)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: assemble handoff: %w", err)
		}

		gapTurns := 0
		if resuming {
			turns, gapErr := rs.conversations.ChatTurns(ctx, chatID)
			if gapErr != nil {
				return "", fmt.Errorf("agent: switch provider: measure handoff gap: %w", gapErr)
			}
			gapTurns = len(turnsAfter(turns, leftAt))
		}

		retry, err := rs.displaceForSwitch(ctx, h, chat)
		if err != nil {
			return "", err
		}
		if retry {
			continue
		}

		return rs.spawnRunner(ctx, chatID, chat.WorkspaceID, targetProviderID, spawnInput{
			launch: &launch, conversation: conversation, gapTurns: gapTurns, resuming: resuming,
		})
	}
}

func (rs *Runners) displaceForSwitch(
	ctx context.Context,
	h *hold,
	chat domain.Chat,
) (bool, error) {
	if _, live := rs.live.byChat(chat.ID); !live {
		return false, nil
	}
	if retry, err := rs.endIdleOutgoing(ctx, chat); retry || err != nil {
		return retry, err
	}
	err := h.outside(func(context.Context) error { return rs.quitOutgoingCLI(ctx, chat.ID) })
	return err == nil, err
}

func (rs *Runners) waitOutPendingPrompt(ctx context.Context, h *hold, chatID string) (waited bool, err error) {
	st, err := rs.settleOrphanedPrompts(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: switch provider: %w", err)
	}
	if len(st.PendingPrompts()) == 0 {
		return false, nil
	}
	return true, h.outside(func(park context.Context) error { return rs.awaitNoPendingPrompt(park, chatID) })
}

func (rs *Runners) endIdleOutgoing(ctx context.Context, chat domain.Chat) (retry bool, err error) {
	rs.live.inTurnOrder(chat.ID, func() {
		if retry, err = rs.outgoingBusy(ctx, chat); !retry && err == nil {
			rs.markOutgoingEnding(chat.ID)
		}
	})
	return retry, err
}

func (rs *Runners) outgoingBusy(
	ctx context.Context,
	chat domain.Chat,
) (bool, error) {
	st, err := rs.settleOrphanedPrompts(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: switch provider: %w", err)
	}
	if len(st.PendingPrompts()) > 0 {
		return true, nil
	}
	if st.Busy() {
		return false, ErrPromptBusy
	}
	return false, nil
}

const WaitingForPendingPromptLog = "agent: switch provider: waiting for the pending prompt"

func (rs *Runners) awaitNoPendingPrompt(park context.Context, chatID string) error {
	logged := false
	for {
		if _, err := rs.settleOrphanedPrompts(park, chatID); err != nil {
			return fmt.Errorf("agent: switch provider: %w", err)
		}
		st, changed, err := rs.ledger.Watch(park, chatID)
		if err != nil {
			return fmt.Errorf("agent: switch provider: watch ledger: %w", err)
		}
		if len(st.PendingPrompts()) == 0 {
			return nil
		}
		if !logged {
			slog.InfoContext(park, WaitingForPendingPromptLog, "chat_id", chatID)
			logged = true
		}
		select {
		case <-changed:
		case <-park.Done():
			return fmt.Errorf("agent: switch provider: waiting for the pending prompt: %w", park.Err())
		}
	}
}

func (rs *Runners) leftAt(ctx context.Context, chatID, providerID, sessionID string) (time.Time, error) {
	if sessionID == "" {
		return time.Time{}, nil
	}
	turns, err := rs.conversations.ChatTurns(ctx, chatID)
	if err != nil {
		return time.Time{}, fmt.Errorf("agent: last turn on the session: %w", err)
	}
	var at time.Time
	for _, t := range turns {
		if t.Provider == providerID && t.SessionID == sessionID && t.At.After(at) {
			at = t.At
		}
	}
	return at, nil
}

func turnsAfter(turns []domain.LedgerTurn, cut time.Time) []domain.LedgerTurn {
	var out []domain.LedgerTurn
	for _, t := range turns {
		if t.At.After(cut) {
			out = append(out, t)
		}
	}
	return out
}
