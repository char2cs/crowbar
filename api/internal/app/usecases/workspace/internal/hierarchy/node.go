package hierarchy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type NodeCreator interface {
	Create(
		ctx context.Context,
		id string,
		kind tree.Kind,
		parentID string,
		order int,
	) (tree.Node, error)
}

type noOpNodeCreator struct{}

func (noOpNodeCreator) Create(
	_ context.Context,
	id string,
	kind tree.Kind,
	parentID string,
	order int,
) (tree.Node, error) {
	return tree.Node{ID: id, Kind: kind, ParentID: parentID, Order: order}, nil
}

func WithNodes(nodes NodeCreator) Option {
	return func(u *hierarchyUsecase) { u.nodes = nodes }
}

func (u *hierarchyUsecase) mintWorkspaceNode(
	ctx context.Context,
	id string,
) error {
	if _, err := u.nodes.Create(ctx, id, tree.KindWorkspace, "", 0); err != nil {
		return fmt.Errorf("hierarchy: mint workspace node: %w", err)
	}
	return nil
}

func (u *hierarchyUsecase) discardWorkspaceRow(
	ctx context.Context,
	id string,
	op string,
) {
	if err := u.workspaces.Delete(ctx, id); err != nil {
		slog.WarnContext(ctx, op+": discard workspace row after failed node mint",
			"workspace_id", id, "err", err)
	}
}
