package lineage

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Resolver reads a chat's lineage out of the stores. It is a read service with
// no writes and no aggregate of its own, which is what lets both the spawn path
// (what a thread must be told to read) and the tool surface (what a chat may not
// read) share one answer instead of each deriving their own.
type Resolver struct {
	folders Folders
	chats   Chats
}

// New builds the resolver over the chat-folder table and the chat repository.
//
// It is deliberately assembled BEFORE the agent usecase rather than hung off the
// Chats-panel tree usecase, which owns the same edges: that usecase already
// holds the agent usecase (a chat delete cascades through it), so reaching back
// the other way for a read would close a construction cycle around two stores
// either of them can simply read.
func New(
	folders Folders,
	chats Chats,
) *Resolver {
	return &Resolver{folders: folders, chats: chats}
}

// Ancestors returns chatID's CHAT ancestors, nearest parent first, with folders
// filtered out. Empty for a chat at the panel root and for one filed in a folder
// — neither inherits anything.
//
// The subject is read through LoadChat, which folds it from the event log. That
// is not interchangeable with the projected read, and the early exit below is
// exactly why: it DECIDES, on this one field, that there is no lineage and that
// none of the reads underneath it are worth doing. Taken on projected state that
// decision is wrong precisely when it matters — straight after the placement was
// written, which is when a create asks — and it returns before the walk that
// would have compensated ever runs. A guard positioned behind an early exit that
// consults the same stale value is not a guard.
//
// A chat with no parent at all costs ONE log fold and no table scans. That
// matters because this runs on every spawn and nearly every chat is such a chat:
// the price of the feature must be paid by the threads that use it, not by every
// chat in the daemon.
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

// lookups reads the workspace's whole Chats tree once and folds it into the two
// maps Walk asks about: every row's container, and which of those rows are
// chats.
//
// The subject chat is stamped in LAST, over whatever the list said about it. The
// list is a projection and can still be serving the placement this chat had
// before the move that prompted the question; the log-folded chat that reached
// this function cannot. Seeding it first would let a stale row overwrite the one
// authoritative answer in the whole map.
func (r *Resolver) lookups(
	ctx context.Context,
	chat domain.Chat,
) (map[string]string, map[string]bool, error) {
	folders, err := r.folders.FindWhere(ctx, domain.ChatFolder{WorkspaceID: chat.WorkspaceID})
	if err != nil {
		return nil, nil, fmt.Errorf("chat lineage: folders: %w", err)
	}
	rows, err := r.chats.ListByWorkspace(ctx, chat.WorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("chat lineage: chats: %w", err)
	}
	parents := make(map[string]string, len(folders)+len(rows)+1)
	chats := make(map[string]bool, len(rows)+1)
	for _, folder := range folders {
		parents[folder.ID] = folder.ParentID
	}
	for _, row := range rows {
		parents[row.ID] = row.ParentID
		chats[row.ID] = true
	}
	parents[chat.ID] = chat.ParentID
	chats[chat.ID] = true
	return parents, chats, nil
}
