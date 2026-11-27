package lineage

import (
	"context"
	"errors"
	"fmt"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Resolver struct {
	chats Chats
}

func New(
	chats Chats,
) *Resolver {
	return &Resolver{chats: chats}
}

func (r *Resolver) Ancestors(
	ctx context.Context,
	chatID string,
) ([]string, error) {
	chat, err := r.chats.LoadChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("chat lineage: chat %s: %w", chatID, err)
	}
	if chat.ParentID == "" {
		return nil, nil
	}
	parents, chats, err := r.lookups(ctx, chat)
	if err != nil {
		return nil, err
	}
	return Walk(
		chatID,
		func(id string) string { return parents[id] },
		func(id string) bool { return chats[id] },
	), nil
}

func (r *Resolver) lookups(
	ctx context.Context,
	chat domain.Chat,
) (map[string]string, map[string]bool, error) {
	rows, err := r.chats.ListByWorkspace(ctx, chat.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("chat lineage: chats: %w", err)
	}
	parents := make(map[string]string, len(rows)+1)
	chats := make(map[string]bool, len(rows)+1)
	for _, row := range rows {
		parents[row.ID] = row.ParentID
		chats[row.ID] = row.IsChat()
	}
	parents[chat.ID] = chat.ParentID
	chats[chat.ID] = chat.IsChat()
	if err := r.fillUnlisted(ctx, chat.ID, parents, chats); err != nil {
		return nil, nil, err
	}
	return parents, chats, nil
}

func (r *Resolver) fillUnlisted(
	ctx context.Context,
	chatID string,
	parents map[string]string,
	chats map[string]bool,
) error {
	seen := map[string]bool{chatID: true}
	for at := parents[chatID]; at != "" && !seen[at]; at = parents[at] {
		seen[at] = true
		if _, listed := parents[at]; listed {
			continue
		}
		row, err := r.chats.LoadChat(ctx, at)
		if errors.Is(err, agentchat.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("chat lineage: ancestor %s: %w", at, err)
		}
		parents[at] = row.ParentID
		chats[at] = row.IsChat()
	}
	return nil
}
