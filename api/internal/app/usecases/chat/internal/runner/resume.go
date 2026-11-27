package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: %w", err)
	}
	defer h.release()

	live, ok, err := rs.settledRunner(h)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: %w", err)
	}
	if ok {
		return live.ID, nil
	}
	providerID, err := rs.lastActiveProviderID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: %w", err)
	}
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseStarting)()
	return rs.switchProviderLocked(ctx, h, chatID, providerID)
}

func (rs *Runners) settledRunner(h *hold) (engineagents.Runner, bool, error) {
	for {
		v, ok := rs.live.view(h.chatID)
		if !ok || !v.exiting {
			return v.runner, ok, nil
		}
		if err := h.outside(func(park context.Context) error {
			_, _, err := rs.awaitDormant(park, h.chatID)
			return err
		}); err != nil {
			return engineagents.Runner{}, false, err
		}
	}
}

func (rs *Runners) lastActiveProviderID(ctx context.Context, chatID string) (string, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}
	if chat.ProviderID == "" {
		return "", ErrChatProviderUnknown
	}
	return chat.ProviderID, nil
}
