package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Chat is the chat aggregate repository.
type Chat interface {
	Create(
		ctx context.Context,
		id string,
		wsID string,
		now time.Time,
	) (domain.Chat, error)
	ResetIdle(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
}

type chat struct {
	ax asynx.Asynx[domain.Chat]
}

// New builds a Chat repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.Chat],
) Chat {
	return &chat{ax: ax}
}

func (c *chat) Create(
	ctx context.Context,
	id string,
	wsID string,
	now time.Time,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.CreateChat{ID: id, WsID: wsID, Now: now})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) ResetIdle(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.ResetChatIdle{ID: id})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: reset idle: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	got, err := c.ax.Get(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: get: %w", err)
	}
	return got, nil
}
