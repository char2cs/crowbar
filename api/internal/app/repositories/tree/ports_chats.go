package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Each chat's own placement, by chat id: what the chat aggregate carried before
// the Tree, read back onto every chat the chat repository returns.

// ChatPlacement is chatID's placement; a chat never placed is at its root, first.
func (p *Ports) ChatPlacement(ctx context.Context, chatID string) (tree.Placement, error) {
	t, ok, err := p.treeOf(ctx, chatID)
	if err != nil || !ok {
		return tree.Placement{}, err
	}
	return t.Chats[chatID], nil
}

// ChatPlacements is the placement of every chat of chatIDs a tree holds,
// reading each tree that holds one once.
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

// SetChatPlacement writes where chatID hangs; workspaceID ("" for none yet)
// names its project.
func (p *Ports) SetChatPlacement(ctx context.Context, chatID, workspaceID string, pl tree.Placement) error {
	at, err := p.placeIn(ctx, chatID, tree.KindChat, workspaceID, pl.ParentID)
	if err != nil {
		return err
	}
	return p.writeTo(ctx, chatID, at, func() error { _, err := p.repo.PutChat(ctx, at, chatID, pl); return err })
}

// SetChatOrder writes chatID's index and keeps its parent as the tree holds it
// when the write lands.
func (p *Ports) SetChatOrder(ctx context.Context, chatID, workspaceID string, order int) error {
	at, err := p.placeIn(ctx, chatID, tree.KindChat, workspaceID, "")
	if err != nil {
		return err
	}
	return p.writeTo(ctx, chatID, at, func() error { _, err := p.repo.OrderChat(ctx, at, chatID, order); return err })
}

// AdoptChat moves chatID's records into its workspace's project once it has
// one: a chat placed before its workspace was attached sat in tree.Unplaced.
func (p *Ports) AdoptChat(ctx context.Context, chatID, workspaceID string) error {
	at, held := p.holder(chatID)
	if !held || at != tree.Unplaced || workspaceID == "" {
		return nil
	}
	_, err := p.placeIn(ctx, chatID, tree.KindChat, workspaceID, "")
	return err
}

// DropChat removes chatID's own placement once the chat is gone.
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
