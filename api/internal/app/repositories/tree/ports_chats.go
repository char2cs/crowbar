package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func (p *Ports) ChatPlacement(ctx context.Context, chatID string) (tree.Placement, error) {
	t, ok, err := p.treeOf(ctx, chatID)
	if err != nil || !ok {
		return tree.Placement{}, err
	}
	return t.Chats[chatID], nil
}

func (p *Ports) ChatPlacements(ctx context.Context, chatIDs []string) (map[string]tree.Placement, error) {
	byTree := map[string][]string{}
	for _, id := range chatIDs {
		if at, ok := p.holder(id); ok {
			byTree[at] = append(byTree[at], id)
		}
	}
	out := map[string]tree.Placement{}
	for at, ids := range byTree {
		t, err := p.repo.Get(ctx, at)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if pl, ok := t.Chats[id]; ok {
				out[id] = pl
			}
		}
	}
	return out, nil
}

func (p *Ports) SetChatPlacement(ctx context.Context, chatID, workspaceID string, pl tree.Placement) error {
	at, err := p.placeIn(ctx, chatID, tree.KindChat, workspaceID, pl.ParentID)
	if err != nil {
		return err
	}
	return p.writeTo(ctx, chatID, at, func() error { _, err := p.repo.PutChat(ctx, at, chatID, pl); return err })
}

func (p *Ports) SetChatOrder(ctx context.Context, chatID, workspaceID string, order int) error {
	at, err := p.placeIn(ctx, chatID, tree.KindChat, workspaceID, "")
	if err != nil {
		return err
	}
	return p.writeTo(ctx, chatID, at, func() error { _, err := p.repo.OrderChat(ctx, at, chatID, order); return err })
}

func (p *Ports) AdoptChat(ctx context.Context, chatID, workspaceID string) error {
	at, held := p.holder(chatID)
	if !held || at != tree.Unplaced || workspaceID == "" {
		return nil
	}
	_, err := p.placeIn(ctx, chatID, tree.KindChat, workspaceID, "")
	return err
}

func (p *Ports) DropChat(ctx context.Context, chatID string) error {
	t, ok, err := p.treeOf(ctx, chatID)
	if err != nil || !ok {
		return err
	}
	if _, exists := t.Chats[chatID]; !exists {
		return nil
	}
	out, err := p.repo.DropChat(ctx, t.ProjectID, chatID)
	if err != nil {
		return err
	}
	p.forgetIfEmpty(chatID, out)
	return nil
}
