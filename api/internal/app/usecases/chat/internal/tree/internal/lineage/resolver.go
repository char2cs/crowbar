package lineage

import (
	"context"
	"errors"
	"fmt"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Resolver reads a chat's lineage out of the store, a read-only service the spawn path and the tool
// surface share so they agree on one answer.
type Resolver struct {
	chats Chats
}

// New builds the resolver over the chat repository. It is assembled before the agent usecase, not hung off
// the tree usecase, because the tree usecase holds the agent usecase and the reverse would be a cycle.
func New(
	chats Chats,
) *Resolver {
	return &Resolver{chats: chats}
}

// Ancestors returns chatID's chat ancestors, nearest first; empty at the root or in a folder. The subject
// comes from LoadChat because the early exit on its ParentID would be wrong on stale projected state right
// after a create; a parentless chat, nearly every one, costs a single log fold.
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

// lookups folds the workspace's rows into Walk's two maps (container, is-chat), stamping the log-folded
// subject last so a stale projected row cannot overwrite the one authoritative answer.
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

// fillUnlisted loads, from the log, every row on chatID's chain the workspace
// list does not carry: a folder owns no workspace, so the list never has it.
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
			return nil // not a chat or folder (a workspace node): the walk ends here
		}
		if err != nil {
			return fmt.Errorf("chat lineage: ancestor %s: %w", at, err)
		}
		parents[at] = row.ParentID
		chats[at] = row.IsChat()
	}
	return nil
}
