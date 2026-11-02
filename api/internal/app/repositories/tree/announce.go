package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// ChatMoved hears a chat's new placement; kind is the chat frame kind it rides
// ("placement_set" when the parent changed, else "order_set").
type ChatMoved func(ctx context.Context, chatID string, p tree.Placement, kind string)

// NodeMoved hears a chat or workspace row whose position Node a Tree write
// changed or minted. A chat with a Node is drawn where its Node is, so its
// own record's moves are not heard.
type NodeMoved func(ctx context.Context, n tree.Node)

// FolderChanged hears a folder a Tree write created, renamed, moved or (gone)
// deleted, at the event's version.
type FolderChanged func(ctx context.Context, projectID string, f tree.Folder, at tree.Node, gone bool, rev int64)

// Announce tells live clients what each Tree write moved, as the chat
// aggregate's own events did before placement left it: chatMoved hears every
// chat whose placement changed, nodeMoved every chat or workspace row whose
// position Node moved, folderChanged every folder that changed.
func Announce(
	ax asynx.Asynx[tree.Tree],
	chatMoved ChatMoved,
	nodeMoved NodeMoved,
	folderChanged FolderChanged,
) error {
	if _, err := ax.Subscribe(asynx.Topic("tree.*"), func(ctx context.Context, evt asynxModels.Event[tree.Tree]) {
		announce(ctx, evt.PreviousAggregate, evt.Aggregate, chatMoved)
		announceNodes(ctx, evt.PreviousAggregate, evt.Aggregate, nodeMoved)
		announceFolders(ctx, evt.PreviousAggregate, evt.Aggregate, folderChanged, evt.Version)
	}); err != nil {
		return fmt.Errorf("tree: announce: %w", err)
	}
	return nil
}

func announce(
	ctx context.Context,
	before tree.Tree,
	after tree.Tree,
	chatMoved ChatMoved,
) {
	for id, p := range after.Chats {
		if _, drawn := after.Nodes[id]; drawn {
			continue
		}
		was, ok := before.Chats[id]
		switch {
		case !ok || was.ParentID != p.ParentID:
			chatMoved(ctx, id, p, "placement_set")
		case was.Order != p.Order:
			chatMoved(ctx, id, p, "order_set")
		}
	}
}

func announceNodes(ctx context.Context, before, after tree.Tree, moved NodeMoved) {
	for id, n := range after.Nodes {
		if n.Kind != tree.KindChat && n.Kind != tree.KindWorkspace {
			continue
		}
		if before.Nodes[id] == n {
			continue
		}
		moved(ctx, n)
	}
}

func announceFolders(ctx context.Context, before, after tree.Tree, changed FolderChanged, rev int64) {
	for id, f := range after.Folders {
		at := after.Nodes[id]
		if was, ok := before.Folders[id]; ok && was == f && before.Nodes[id] == at {
			continue
		}
		changed(ctx, after.ProjectID, f, at, false, rev)
	}
	for id, f := range before.Folders {
		if _, ok := after.Folders[id]; !ok {
			changed(ctx, before.ProjectID, f, before.Nodes[id], true, rev)
		}
	}
}
