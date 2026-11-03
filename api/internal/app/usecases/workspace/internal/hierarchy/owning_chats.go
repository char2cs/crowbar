package hierarchy

import (
	"context"
	"errors"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ImportedBranch is one branch to materialise chat-first in one call. Every field is explicit because the
// batch import already resolved it; ParentWorkspaceID is the git lineage parent, while sidebar placement
// follows from the chat owning it.
type ImportedBranch struct {
	// ChatID is the chat to mint for the branch; "" mints one.
	ChatID            string
	RepoID            string
	ProjectID         string
	RepoPath          string
	RemoteURL         string
	Branch            string
	ParentWorkspaceID string
	ParentBranch      string
	ForceLocked       bool
}

// OwningChats is the chat-side surface that keeps every workspace born under its owning chat, declared by
// the consumer because chat and workspace usecases are siblings. ImportBranchAsChat covers a real
// worktree end to end; the three verbs below serve the placeholder, which builds its own row.
type OwningChats interface {
	// ImportBranchAsChat mints the owning chat, creates the workspace for an
	// existing branch, attaches the two, and returns both ids. A failure after
	// the mint still returns the chat, placed, for the caller's placeholder.
	ImportBranchAsChat(
		ctx context.Context,
		in ImportedBranch,
	) (chatID, workspaceID string, err error)
	// MintOwningChat mints (as chatID, or a fresh id for "") and places the chat
	// about to own a workspace the caller builds, under parentWorkspaceID's chat.
	MintOwningChat(
		ctx context.Context,
		chatID string,
		parentWorkspaceID string,
	) (string, error)
	// OwnerOf is the chat that owns workspaceID, "" when none does.
	OwnerOf(
		ctx context.Context,
		workspaceID string,
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

// ErrNoOwningChats refuses an import on a usecase whose chat side was never wired, rather than create a
// workspace owned by nothing and unreachable from the sidebar.
var ErrNoOwningChats = errors.New("hierarchy: no owning-chat surface wired; refusing to create a chat-less workspace")

// discardOwningChat takes back a chat minted for a workspace that failed to create and returns the
// original cause. Best-effort: a leftover workspace-less chat is visible and deletable by hand.
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

// discardUnownedWorkspace rolls back both halves when the row could not be attached to its chat: the
// workspace first, while nothing claims it, so no unreachable orphan is left.
func (u *hierarchyUsecase) discardUnownedWorkspace(
	ctx context.Context,
	chatID string,
	ws domain.Workspace,
	cause error,
) error {
	if err := u.removeOne(ctx, ws, u.repoRefFor(ctx, ws.RepoID), nil); err != nil {
		slog.WarnContext(ctx, "import: discard the workspace no chat came to own",
			"workspace_id", ws.ID, "err", err)
	}
	return u.discardOwningChat(ctx, chatID, cause)
}
