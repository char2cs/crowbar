package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// OwningChats is the narrow chat-side surface this usecase needs so that no
// workspace a repo import creates can exist without the chat that owns it.
//
// Declared HERE, by the consumer (the same rule chat.WorktreeCreator and
// hierarchy.OwningChats follow): usecases/project and usecases/chat are
// siblings, neither imports the other, and the container satisfies this with
// whatever concrete type actually implements it.
//
// It is the three-verb shape rather than the hierarchy's one-call import,
// because every workspace this usecase creates is one it builds ITSELF and in a
// way no generic import could: the repo home is adopted in place on the folder
// the user already had, a protected branch is checked out at origin's ref with
// its upstream set, and a held branch gets a row with no worktree at all. What
// they lacked was never the git work — it was the chat.
type OwningChats interface {
	// MintOwningChat mints and places the chat that is about to own a workspace,
	// under the chat owning parentWorkspaceID (empty for a repo-tree root).
	MintOwningChat(
		ctx context.Context,
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

// ErrNoOwningChats is returned when a repo import runs on a usecase whose chat
// side was never wired.
//
// It refuses rather than falling back to the workspace-only create it used to
// do, and that is the point: a create that quietly proceeded without a chat is
// how a branch ended up on disk owned by nothing (spec §0). An unwired daemon
// fails at its first import instead of producing rows nothing can address.
var ErrNoOwningChats = errors.New("project import: no owning-chat surface wired; refusing to create a chat-less workspace")

// createOwnedWorkspace is the chat-first create every workspace-making path in
// this usecase goes through.
//
// The order is the whole contract: the chat is minted BEFORE the row exists,
// and if the row cannot be written, or cannot be attached, neither survives. A
// caller hands it the row it wants and the git-lineage parent that row hangs
// off; where the new chat is PLACED follows from the chat owning that parent,
// which is the chat side's join to make.
func (u *projectImport) createOwnedWorkspace(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	if u.owningChats == nil {
		return domain.Workspace{}, ErrNoOwningChats
	}
	chatID, err := u.owningChats.MintOwningChat(ctx, in.ParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("mint owning chat: %w", err)
	}
	ws, err := u.deps.Workspaces.Create(ctx, in, now)
	if err != nil {
		return domain.Workspace{}, u.discardOwningChat(ctx, chatID, err)
	}
	if aErr := u.owningChats.AttachOwningWorkspace(ctx, chatID, ws); aErr != nil {
		return domain.Workspace{}, u.discardUnownedWorkspace(ctx, chatID, ws.ID, aErr)
	}
	return ws, nil
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

// discardUnownedWorkspace rolls back both halves when the row was written but
// could not be attached to the chat minted for it. The workspace goes first,
// while nothing claims it, then the chat — a row left behind here would be the
// exact orphan this change exists to make unrepresentable.
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
	return u.discardOwningChat(ctx, chatID, cause)
}
