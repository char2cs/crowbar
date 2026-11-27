package tree

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type FolderReader interface {
	FindByKey(ctx context.Context, id string) (*dtree.Folder, error)
}

func FoldersBelow(
	ctx context.Context, folders FolderReader, byParent map[string][]dtree.Node, rows []domain.Chat,
) ([]domain.Chat, error) {
	if folders == nil {
		return nil, nil
	}
	var found []domain.Chat
	queried := map[string]bool{}
	seen := map[string]bool{}
	queue := make([]string, 0, len(rows)+1)
	queue = append(queue, "")
	for _, row := range rows {
		queue = append(queue, row.ID)
	}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		if queried[parent] {
			continue
		}
		queried[parent] = true
		for _, n := range byParent[parent] {
			if n.Kind != dtree.KindFolder || seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			f, err := folders.FindByKey(ctx, n.ID)
			if err != nil {
				return nil, err
			}
			if f == nil {
				continue
			}
			found = append(found, domain.Chat{
				ID: f.ID, Type: domain.ChatTypeFolder, RepoID: f.RepoID,
				Title: f.Name, ParentID: n.ParentID, Order: n.Order,
			})
			queue = append(queue, n.ID)
		}
	}
	return found, nil
}
