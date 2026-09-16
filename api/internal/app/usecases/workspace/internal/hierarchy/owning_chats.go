package hierarchy

import (
	"context"
	"errors"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ImportedBranch is one branch this usecase asks to have materialised
// chat-first: the chat minted and placed, the workspace created and attached to
// it, in one call.
//
// Every field is named outright rather than left to the worktree create's own
// parent-inherited defaulting, because the batch import already resolved all of
// it — it walked one repository's open-PR graph to get here. ParentWorkspaceID
// is the GIT lineage parent the chain walk resolved (domain.Workspace.ParentID);
// where the new row is PLACED in the sidebar follows from the chat that owns
// that workspace, which is the chat side's join to make, not this one's.
type ImportedBranch struct {
	RepoID            string
	ProjectID         string
	RepoPath          string
	RemoteURL         string
	Branch            string
	ParentWorkspaceID string
	ParentBranch      string
	ForceLocked       bool
}

// OwningChats is the narrow chat-side surface this usecase needs so that no
// workspace it creates can exist without the chat that owns it.
//
// Declared HERE, by the consumer, for the reason every other port in this
// daemon is (see chat.WorktreeCreator's own doc, the mirror of this one
// pointing the other way): usecases/chat and usecases/workspace are siblings
// and neither imports the other, so each names the narrow slice of the other it
// needs and the container satisfies it.
//
// It carries two shapes because this usecase creates workspaces two ways.
// ImportBranchAsChat is the whole sequence for a branch that git can give a
// real worktree — the chat side owns it end to end. The three verbs below it
// are for the PLACEHOLDER, where there is no worktree to create and the row
// exists precisely to record why: that path builds its own row and only needs
// the chat minted before it, and the rollback kept where the knowledge is.
type OwningChats interface {
	// ImportBranchAsChat mints the owning chat, creates the workspace for an
	// existing branch, attaches the two, and returns the workspace id. A failure
	// anywhere takes the chat back out.
	ImportBranchAsChat(
		ctx context.Context,
		in ImportedBranch,
	) (workspaceID string, err error)
	// MintOwningChat mints and places the chat that is about to own a workspace
	// the caller builds itself, under the chat owning parentWorkspaceID.
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
	// DiscardOwningChat takes a minted owning chat back out when the caller's
	// own workspace creation then failed.
	DiscardOwningChat(
		ctx context.Context,
		chatID string,
	) error
}

// ErrNoOwningChats is returned when an import runs on a usecase whose chat side
// was never wired.
//
// It is a refusal rather than a fall-back to the old workspace-only create, and
// that is the entire point of this change: a create that quietly proceeded
// without minting a chat is exactly how a branch ended up on disk owned by
// nothing and reachable from nowhere (spec §0). An unwired daemon must fail
// loudly at the first import, not produce rows the sidebar cannot address.
var ErrNoOwningChats = errors.New("hierarchy: no owning-chat surface wired; refusing to create a chat-less workspace")

// discardOwningChat takes back a chat minted for a workspace that then failed
// to be created, and hands back the failure that caused it.
//
// The purge is best-effort and never replaces the cause, the same shape every
// other half-create rollback in this daemon uses: what failed is the import,
// and that is what the caller is told. A purge that itself fails leaves a
// placed, workspace-less chat — visible in the panel and deletable by hand,
// which is a far better outcome than a workspace nothing owns.
func (u *hierarchyUsecase) discardOwningChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.owningChats.DiscardOwningChat(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "import: discard the owning chat of a workspace that was never created",
			"chat_id", chatID, "err", err)
	}
	return cause
}

// discardUnownedWorkspace rolls back BOTH halves when the row was written but
// could not be attached to the chat minted for it.
//
// The order is the whole point: the workspace goes first, while nothing claims
// it, and the chat second. A row left behind here is precisely the orphan this
// change exists to make unrepresentable — reachable by nothing, because the
// only door to a workspace is the chat that owns it.
func (u *hierarchyUsecase) discardUnownedWorkspace(
	ctx context.Context,
	chatID string,
	ws domain.Workspace,
	cause error,
) error {
	if err := u.removeOne(ctx, ws, ""); err != nil {
		slog.WarnContext(ctx, "import: discard the workspace no chat came to own",
			"workspace_id", ws.ID, "err", err)
	}
	return u.discardOwningChat(ctx, chatID, cause)
}
