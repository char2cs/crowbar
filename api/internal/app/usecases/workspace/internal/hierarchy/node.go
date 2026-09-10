package hierarchy

import (
	"context"
	"fmt"
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

// noOpNodeCreator is the default every hierarchyUsecase is built with: it
// succeeds trivially and mints nothing durable, so the ~180 existing test
// call sites that construct a usecase with no WithNodes option at all keep
// passing unchanged. u.nodes is therefore NEVER nil — see New — which is what
// lets mintWorkspaceNode treat every error Create returns as real rather than
// having to special-case "unwired".
//
// PRODUCTION always overrides this with a real Node repo (workspace.WithNodes
// in usecases/container.go), so this default is reached only by a test that
// genuinely does not care about Node minting; one that does wires its own
// NodeCreator via WithNodes instead, real or failing.
type noOpNodeCreator struct{}

func (noOpNodeCreator) Create(
	_ context.Context,
	id string,
	kind domain.NodeKind,
	parentID string,
	order int,
) (domain.Node, error) {
	return domain.Node{ID: id, Kind: kind, ParentID: parentID, Order: order}, nil
}

// WithNodes wires the Node surface every workspace this usecase creates mints
// its own position row through, overriding the noOpNodeCreator default (see
// its own doc). The composition root always passes a real Node repo here for
// the running daemon.
func WithNodes(nodes NodeCreator) Option {
	return func(u *hierarchyUsecase) { u.nodes = nodes }
}

// mintWorkspaceNode mints wsID's own position row and returns any failure to
// the caller, which rolls the WHOLE create back on it (2026-09-08
// sidebar-placement-unification Task 7 review fix).
//
// This used to be best-effort — log a warning, let the create stand — on the
// theory that nothing reads a workspace's own Node row yet, so a gap was
// invisible. That reasoning missed that u.nodes.Create can fail in a
// perfectly healthy, fully-wired running daemon, independent of any
// misconfiguration: it runs AFTER a potentially-slow git worktree operation,
// so a client disconnect/request-timeout cancelling ctx in that window fails
// it as a direct consequence, and a transient store-write error at exactly
// the moment after workspaces.Create already committed produces the same
// silent outcome. Unlike the old EnsureOwningChat's own "never fail the
// write" contract, there is no boot-time backfill catching stragglers here —
// building one would be a Node-side backfill, which is not this repo's
// convention (pre-production: graceful fallback for PRE-EXISTING data, never
// a migration/backfill for state created going forward). So every caller
// below now rolls back instead: a Workspace row must never survive with no
// Node row of its own.
func (u *hierarchyUsecase) mintWorkspaceNode(
	ctx context.Context,
	id string,
) error {
	if _, err := u.nodes.Create(ctx, id, domain.NodeKindWorkspace, "", 0); err != nil {
		return fmt.Errorf("hierarchy: mint workspace node: %w", err)
	}
	return nil
}

// discardWorkspaceRow best-effort tombstones a workspace row created but then
// abandoned because its own Node row could not be minted, so a caller with no
// worktree/branch of its own to unwind still leaves nothing but the tombstone
// behind — a row with no Node row must never survive. Logged, never returned:
// the mint failure is the cause the caller reports, not this cleanup.
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
