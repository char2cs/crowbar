package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// OwningChats is the chat-side surface ensuring no workspace a repo import creates exists without its
// owning chat. Declared by the consumer (project and chat are siblings). Three verbs, not one import
// call, because this usecase builds each workspace itself in ways no generic import could.
type OwningChats interface {
	// MintOwningChat mints and places the chat that is about to own a workspace,
	// under the chat owning parentWorkspaceID (empty for a repo-tree root).
	MintOwningChat(
		ctx context.Context,
		chatID string,
		parentWorkspaceID string,
	) (string, error)
	// AttachOwningWorkspace points a minted owning chat at the workspace it was
	// minted for.
	AttachOwningWorkspace(
		ctx context.Context,
		chatID string,
		ws domain.Workspace,
	) error
	// DiscardOwningChat takes a minted owning chat back out when the workspace
	// it was minted for could not be created.
	DiscardOwningChat(
		ctx context.Context,
		chatID string,
	) error
}

// ErrNoOwningChats is returned when a repo import runs with no chat side wired. It refuses rather
// than create a workspace owned by nothing, so an unwired daemon fails at its first import.
var ErrNoOwningChats = errors.New("project import: no owning-chat surface wired; refusing to create a chat-less workspace")

// createOwnedWorkspace is the chat-first create every workspace path here uses: the chat is minted
// before the row, and if the row cannot be written or attached neither survives. Placement follows
// from the chat owning the git-lineage parent.
func (u *projectImport) createOwnedWorkspace(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	if u.owningChats == nil {
		return domain.Workspace{}, ErrNoOwningChats
	}
	chatID, err := u.owningChats.MintOwningChat(ctx, "", in.ParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("mint owning chat: %w", err)
	}
	ws, err := u.deps.Workspaces.Create(ctx, in, now)
	if err != nil {
		return domain.Workspace{}, u.discardOwningChat(ctx, chatID, err)
	}
	// Every workspace mints its own Node row at creation, as a repo does, filed at the root; nothing
	// places it for real yet.
	if nErr := u.mintWorkspaceNode(ctx, ws.ID); nErr != nil {
		return domain.Workspace{}, u.discardUnownedWorkspace(ctx, chatID, ws.ID, nErr)
	}
	if aErr := u.owningChats.AttachOwningWorkspace(ctx, chatID, ws); aErr != nil {
		return domain.Workspace{}, u.discardUnownedWorkspace(ctx, chatID, ws.ID, aErr)
	}
	return ws, nil
}

// mintWorkspaceNode mints wsID's own position row, refusing rather than persisting a workspace with
// no Node row when the surface was never wired.
func (u *projectImport) mintWorkspaceNode(
	ctx context.Context,
	wsID string,
) error {
	if u.deps.Nodes == nil {
		return ErrNoNodesWired
	}
	if _, err := u.deps.Nodes.Create(ctx, wsID, tree.KindWorkspace, "", 0); err != nil {
		return fmt.Errorf("mint workspace node: %w", err)
	}
	return nil
}

// discardOwningChat takes back a chat minted for a workspace that then failed
// to be created, and hands back the failure that caused it. Best-effort, and it
// never replaces the cause: what failed is the import.
func (u *projectImport) discardOwningChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.owningChats.DiscardOwningChat(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "project import: discard the owning chat of a workspace that was never created",
			"chat_id", chatID, "err", err)
	}
	return cause
}

// discardUnownedWorkspace rolls back a create that could not finish: workspace first (while nothing
// claims it), then its Node row (best-effort), then the chat — so no orphan is left.
func (u *projectImport) discardUnownedWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
	cause error,
) error {
	if err := u.deps.Workspaces.Delete(ctx, workspaceID); err != nil {
		slog.WarnContext(ctx, "project import: discard the workspace no chat came to own",
			"workspace_id", workspaceID, "err", err)
	}
	if u.deps.Nodes != nil {
		if err := u.deps.Nodes.Forget(ctx, workspaceID); err != nil {
			slog.WarnContext(ctx, "project import: discard the node row of a workspace no chat came to own",
				"workspace_id", workspaceID, "err", err)
		}
	}
	return u.discardOwningChat(ctx, chatID, cause)
}
