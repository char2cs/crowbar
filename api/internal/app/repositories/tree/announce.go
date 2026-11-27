package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type ChatMoved func(ctx context.Context, chatID string, p tree.Placement)

type NodeMoved func(ctx context.Context, n tree.Node)

type FolderChanged func(ctx context.Context, projectID string, f tree.Folder, at tree.Node, gone bool, rev int64)

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
		if was, ok := before.Chats[id]; !ok || was != p {
			chatMoved(ctx, id, p)
		}
	}
}

func announceNodes(ctx context.Context, before, after tree.Tree, moved NodeMoved) {
	for id, n := range after.Nodes {
		if n.Kind != tree.KindChat && n.Kind != tree.KindWorkspace && n.Kind != tree.KindRepo {
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
