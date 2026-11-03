package hierarchy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// NodeCreator is the write-only Node surface that mints a workspace's own position row when
// CreateChild/CreateFromImport create it.
type NodeCreator interface {
	Create(
		ctx context.Context,
		id string,
		kind tree.Kind,
		parentID string,
		order int,
	) (tree.Node, error)
}

// noOpNodeCreator is the default, minting nothing, for tests that do not care about Nodes; it keeps
// u.nodes never nil so every Create error is real. Production always wires a real repo via WithNodes.
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

// WithNodes wires the Node surface every created workspace mints its position row through,
// overriding noOpNodeCreator; the composition root always passes the real repo.
func WithNodes(nodes NodeCreator) Option {
	return func(u *hierarchyUsecase) { u.nodes = nodes }
}

// mintWorkspaceNode mints wsID's own position row; callers roll the whole create back on failure.
// Create can fail in a healthy daemon (ctx cancelled after a slow git op), and no backfill exists, so
// a Workspace row must never survive without its Node row.
func (u *hierarchyUsecase) mintWorkspaceNode(
	ctx context.Context,
	id string,
) error {
	if _, err := u.nodes.Create(ctx, id, tree.KindWorkspace, "", 0); err != nil {
		return fmt.Errorf("hierarchy: mint workspace node: %w", err)
	}
	return nil
}

// discardWorkspaceRow best-effort tombstones a workspace row whose Node could not be minted. Logged,
// never returned: the mint failure is the cause the caller reports.
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
