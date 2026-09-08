package hierarchy

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// NodeCreator is the narrow write-only Node surface this usecase needs to mint
// a workspace's OWN position row the instant CreateChild/CreateFromImport
// create one (2026-09-08 sidebar-placement-unification Task 7) — the
// hierarchy-package mirror of project.NodePlacements' own Create.
type NodeCreator interface {
	Create(
		ctx context.Context,
		id string,
		kind domain.NodeKind,
		parentID string,
		order int,
	) (domain.Node, error)
}

// WithNodes wires the Node surface every workspace this usecase creates mints
// its own position row through. Optional, unlike project.ImportDeps.Nodes: the
// composition root always supplies a real one for the running daemon, but
// nothing yet reads a workspace's own Node row (its sidebar placement is still
// carried by its owning chat's row — see the wire-DTO's owningChatId), so an
// unwired mintWorkspaceNode degrades to a logged warning here rather than
// failing the create outright, unlike the project package's harder
// ErrNoNodesWired refusal for a repo's Node row (which downstream densify
// passes already depend on existing).
func WithNodes(nodes NodeCreator) Option {
	return func(u *hierarchyUsecase) { u.nodes = nodes }
}

// mintWorkspaceNode mints wsID's own position row, best-effort: nothing reads
// it yet, so a missing surface or a failed write is logged and the workspace
// create it rode in on still stands, mirroring EnsureOwningChat's own
// never-fail-the-write contract for a secondary reconciliation write.
func (u *hierarchyUsecase) mintWorkspaceNode(
	ctx context.Context,
	id string,
) {
	if u.nodes == nil {
		return
	}
	if _, err := u.nodes.Create(ctx, id, domain.NodeKindWorkspace, "", 0); err != nil {
		slog.WarnContext(ctx, "hierarchy: mint workspace node (best-effort, continuing)",
			"workspace_id", id, "err", err)
	}
}
