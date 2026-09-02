package tree

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// createImportedWorktreeChat is CreateChat's WorktreeImport branch, and it is
// deliberately the SAME shape as createOwnWorktreeChat (chats.go): mint the
// chat, place it, and only then attach a worktree to it.
//
// Import used to be the one worktree-provisioning path that did none of this.
// It created a workspace row straight from a discovered git branch, in another
// usecase, with no chat minted in the same breath — so a branch could land on
// disk owned by nothing, addressable by nothing, and visible nowhere (spec §0).
// Reusing this scaffold is what makes that unrepresentable rather than merely
// reconciled after the fact: the chat exists BEFORE the workspace does, and a
// failure anywhere after the mint takes the chat back out.
//
// The row is retyped to a BRANCH row when the workspace it ended up owning is
// one the sidebar draws as a branch — a locked branch, most often, which is
// what every protected import produces. That judgement is not restated here; it
// is owningChatType (owning_rows.go), the same rule the boot backfill takes.
func (u *chatFolderUsecase) createImportedWorktreeChat(
	ctx context.Context,
	providerID string,
	parentID string,
	spec ImportSpec,
) (string, domain.Workspace, string, error) {
	parentID, err := u.importPlacement(ctx, parentID, spec)
	if err != nil {
		return "", domain.Workspace{}, "", err
	}
	if parentID != "" {
		if err := u.checkNewChatParent(ctx, "", parentID, true); err != nil {
			return "", domain.Workspace{}, "", err
		}
	}
	chatID, err := u.agent.MintChat(ctx, "")
	if err != nil {
		return "", domain.Workspace{}, "", fmt.Errorf("agent chat folder: import chat: %w", err)
	}
	// Filed from a global count of its level rather than through placeChat: an
	// imported row is an OWNING row, and it leaves the workspace-less scope
	// placeChat plans against as soon as its worktree is attached. See
	// placeOwningRow (owning_chat.go).
	if pErr := u.placeOwningRow(ctx, chatID, parentID); pErr != nil {
		return "", domain.Workspace{}, "", u.discard(ctx, chatID, pErr)
	}
	ws, runnerID, err := u.agent.SpawnChatWithImportedWorktree(ctx, chatID, providerID, spec)
	if err != nil {
		return "", domain.Workspace{}, "", u.discard(ctx, chatID, err)
	}
	u.retypeOwningRow(ctx, chatID, ws)
	return chatID, ws, runnerID, nil
}

// ImportBranchAsChat is createImportedWorktreeChat as a BATCH importer needs
// it: no provider (so no vendor CLI is launched for a row nobody has opened),
// no explicit chat parent (the placement follows the spec's git lineage), and
// the workspace id handed straight back.
//
// The id comes from the create itself rather than from a read-back of the chat
// that now owns it, and that is deliberate: the chat read model is an
// asynchronous projection, so a lookup taken immediately after this write can
// still be serving the row as it stood before the workspace was attached.
func (u *chatFolderUsecase) ImportBranchAsChat(
	ctx context.Context,
	spec ImportSpec,
) (string, string, error) {
	chatID, ws, _, err := u.createImportedWorktreeChat(ctx, "", "", spec)
	if err != nil {
		return "", "", err
	}
	return chatID, ws.ID, nil
}

// importPlacement answers where an imported row is BORN, from the only thing a
// batch importer actually knows: the git lineage.
//
// An explicit parentID always wins — that is a caller naming a chat outright.
// Otherwise the spec's ParentWorkspaceID is translated into the chat that owns
// it, which is the one join §7.6 leaves to this side. A batch import resolves
// its parents as WORKSPACES (a PR base branch is a branch, not a conversation),
// and the sidebar places rows under CHATS; keeping the translation here means
// no caller has to learn it, and the workspace-lineage maps a chain walk
// already builds stay exactly as they are.
func (u *chatFolderUsecase) importPlacement(
	ctx context.Context,
	parentID string,
	spec ImportSpec,
) (string, error) {
	if parentID != "" {
		return parentID, nil
	}
	return u.owningChatOf(ctx, spec.ParentWorkspaceID)
}

// retypeOwningRow makes a just-attached row the BRANCH row its workspace is
// owed, when that is what the workspace turns out to be.
//
// It is best-effort and never fails the create. By the time it runs the branch
// is imported, the workspace exists and the chat owns it — every invariant this
// path exists to establish already holds — and the only thing left is which
// glyph the sidebar draws. Undoing a completed import over that would destroy
// real work, and the boot backfill takes exactly this decision again anyway
// (see adopt, owning_rows.go), so a row that misses it here is repaired rather
// than stranded.
func (u *chatFolderUsecase) retypeOwningRow(
	ctx context.Context,
	chatID string,
	ws domain.Workspace,
) {
	if owningChatType(ws) != domain.ChatTypeBranch {
		return
	}
	if _, err := u.chats.SetType(ctx, chatID, domain.ChatTypeBranch); err != nil {
		slog.WarnContext(ctx, "agent chat folder: retype imported row as a branch row (best-effort, continuing)",
			"chat_id", chatID, "workspace_id", ws.ID, "err", err)
	}
}
