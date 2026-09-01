package tree

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The chat half of the tree: placing a chat, and the cascade that takes its
// threads with it when it goes.

func (u *chatFolderUsecase) CreateChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
	parentID string,
	ownWorktree bool,
) (string, string, error) {
	if ownWorktree {
		return u.createOwnWorktreeChat(ctx, providerID, parentID)
	}
	if parentID == "" {
		return u.agent.SpawnChat(ctx, workspaceID, providerID)
	}
	if err := u.checkNewChatParent(ctx, workspaceID, parentID, false); err != nil {
		return "", "", err
	}
	chatID, err := u.agent.MintChat(ctx, workspaceID)
	if err != nil {
		return "", "", fmt.Errorf("agent chat folder: create chat: %w", err)
	}
	if _, _, pErr := u.PlaceChat(ctx, workspaceID, chatID, PlaceInput{ParentID: &parentID}); pErr != nil {
		return "", "", u.discard(ctx, chatID, pErr)
	}
	runnerID, err := u.agent.StartRunner(ctx, chatID, providerID)
	if err != nil {
		return "", "", u.discard(ctx, chatID, err)
	}
	return chatID, runnerID, nil
}

// createOwnWorktreeChat is CreateChat's ownWorktree branch. It mints the chat
// and places it under parentID exactly as the plain-bubble path above does —
// always in the workspace-less ("") scope, since the row IS a plain bubble
// until agent.SpawnChatWithOwnWorktree fills its slot — then, instead of a bare
// StartRunner, forks a fresh worktree from the chat's own resolved fork parent
// and starts providerID's CLI in it.
//
// Reusing the workspace-less scope for placement (rather than inventing one
// keyed on the not-yet-minted workspace) is deliberate: checkNewChatParent's
// same-workspace rule then applies exactly as it already does to an ordinary
// bubble thread, EXCEPT that this caller passes ownWorktree=true, so a parent
// that already owns a worktree of its own — a CHAT as well as a BRANCH — is
// also an acceptable fork point (model spec §4.1: "under a row that carries a
// branch the new row is a worktree"). A folder or another workspace-less chat
// remains acceptable too; only a nonexistent parent is refused.
//
// Placement happens BEFORE the workspace is minted, mirroring the thread
// path's own reason: a chat's lineage — and here, the fork parent its walk
// resolves — must be fixed before its first CLI exists.
func (u *chatFolderUsecase) createOwnWorktreeChat(
	ctx context.Context,
	providerID string,
	parentID string,
) (string, string, error) {
	if parentID != "" {
		if err := u.checkNewChatParent(ctx, "", parentID, true); err != nil {
			return "", "", err
		}
	}
	chatID, err := u.agent.MintChat(ctx, "")
	if err != nil {
		return "", "", fmt.Errorf("agent chat folder: create chat: %w", err)
	}
	if parentID != "" {
		if _, _, pErr := u.placeChat(ctx, "", chatID, PlaceInput{ParentID: &parentID}, true); pErr != nil {
			return "", "", u.discard(ctx, chatID, pErr)
		}
	}
	runnerID, err := u.agent.SpawnChatWithOwnWorktree(ctx, chatID, providerID)
	if err != nil {
		return "", "", u.discard(ctx, chatID, err)
	}
	return chatID, runnerID, nil
}

// checkNewChatParent refuses a destination a new chat cannot be born into, BEFORE
// anything is minted: a row that does not exist, or (for an ordinary thread) a
// CHAT belonging to another workspace. It is the same guard a move makes, minus
// the cycle test — a chat that does not exist yet cannot be inside its own
// subtree.
//
// ownWorktree is threaded straight through to checkChatContainer: it is true
// only for createOwnWorktreeChat's own call, and it is what lets that caller
// name a worktree-owning CHAT parent the same-workspace rule would otherwise
// refuse.
//
// It runs first so the ordinary failure costs nothing. Leaving it to PlaceChat
// would mint a chat, broadcast it, refuse the placement and then purge it again,
// which is a create and a delete on every mistyped id.
func (u *chatFolderUsecase) checkNewChatParent(
	ctx context.Context,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	snapshot, err := u.workspaceSnapshot(ctx, workspaceID)
	if err != nil {
		return err
	}
	return u.checkChatContainer(ctx, snapshot, workspaceID, parentID, ownWorktree)
}

// discard takes a just-minted chat back out when the create failed after minting
// it, and hands back the failure that caused it.
//
// The purge is best-effort and NEVER replaces the cause. The user asked to create
// a chat and that is what failed, so that is what they are told; a purge that
// fails on top of it leaves a placed, CLI-less chat, which is visible in the panel
// and deletable by hand — a far better outcome than reporting an error other than
// the one that actually happened.
func (u *chatFolderUsecase) discard(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.agent.PurgeChat(ctx, chatID); err != nil {
		slog.WarnContext(ctx, "agent chat folder: discard half-created chat",
			"err", err, "chat_id", chatID)
	}
	return cause
}

func (u *chatFolderUsecase) PlaceChat(
	ctx context.Context,
	workspaceID string,
	chatID string,
	in PlaceInput,
) (domain.Chat, []domain.Chat, error) {
	return u.placeChat(ctx, workspaceID, chatID, in, false)
}

// placeChat is PlaceChat's body, taking one extra argument PlaceChat's own
// exported signature does not carry: ownWorktree, true only for
// createOwnWorktreeChat's own call. That caller's placement is a second
// destination check on the SAME parentID checkNewChatParent already cleared
// (loadChat/workspaceSnapshotAround resolve a fresh snapshot, so the check is
// re-run rather than trusted), and it has to see the same ownWorktree signal
// checkNewChatParent did or it refuses the exact case Task 7 exists to allow.
func (u *chatFolderUsecase) placeChat(
	ctx context.Context,
	workspaceID string,
	chatID string,
	in PlaceInput,
	ownWorktree bool,
) (domain.Chat, []domain.Chat, error) {
	current, err := u.loadChat(ctx, workspaceID, chatID)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	snapshot, err := u.workspaceSnapshotAround(ctx, workspaceID, current)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if mErr := u.checkChatMove(ctx, snapshot, workspaceID, chatID, destination, ownWorktree); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if wErr := guardNotWorking(subtreeIDsOf(chatID, snapshot.rows), u.work); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	// Read BEFORE the plan is mutated: this is the only moment the lineage the
	// chat has been living under is still recoverable, and the comparison against
	// what it lands on is what decides whether anything happened worth recording.
	inherited := snapshot.chatLineage(chatID)
	u.replace(snapshot, chatID, current.ParentID, destination, in.Order)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	u.noteNewAncestors(ctx, chatID, inherited, snapshot.chatLineage(chatID))
	return *snapshot.placedRow(chatID), written, nil
}

// noteNewAncestors writes the move into the chat's own conversation when, and
// only when, it actually gained a chat to read.
//
// GAINED, not merely changed. A chat dragged between two folders under the same
// parent reads exactly what it read a moment ago — folders are transparent — and
// announcing a context change there would be announcing one that did not happen.
// A chat dragged OUT from under a parent gains nothing and is likewise silent:
// what it no longer reads is already answered by its next spawn resolving an
// empty lineage, and there is no new context to date the start of.
//
// A failed note is logged and never returned. The rows have already moved by the
// time this runs, so failing the drag here would report an error for a move that
// happened and stands; and the relationship itself rides on ParentID, so what is
// lost is the line in the record, not the behaviour it describes.
func (u *chatFolderUsecase) noteNewAncestors(
	ctx context.Context,
	chatID string,
	inherited []string,
	lineage []string,
) {
	if !gained(inherited, lineage) {
		return
	}
	if err := u.agent.NoteThreadLineage(ctx, chatID, lineage); err != nil {
		slog.WarnContext(ctx, "agent chat folder: record new thread lineage",
			"err", err, "chat_id", chatID)
	}
}

// gained reports whether lineage names a chat that inherited did not — the test
// for "this chat now reads something it was not reading before".
func gained(
	inherited []string,
	lineage []string,
) bool {
	had := make(map[string]bool, len(inherited))
	for _, id := range inherited {
		had[id] = true
	}
	return slices.ContainsFunc(lineage, func(id string) bool { return !had[id] })
}

// DeleteChat erases chatID and every chat threaded below it. A FOLDER id is
// refused as not-found rather than served: the folder verb PROMOTES what it
// held and this one CASCADES into it, so accepting a folder here would erase
// every chat filed inside one on a route that only ever meant to delete a
// conversation. See loadChat's own doc for the other half of the same guard.
func (u *chatFolderUsecase) DeleteChat(
	ctx context.Context,
	chatID string,
) (ChatDeletion, error) {
	current, err := u.chats.LoadChat(ctx, chatID)
	if err != nil {
		return ChatDeletion{}, fmt.Errorf("agent chat folder: delete chat %s: %w", chatID, err)
	}
	if current.Type == domain.ChatTypeFolder {
		return ChatDeletion{}, fmt.Errorf("agent chat folder: %s is a folder: %w", chatID, apperr.ErrNotFound)
	}
	snapshot, err := u.workspaceSnapshotAround(ctx, current.WorkspaceID, current)
	if err != nil {
		return ChatDeletion{}, err
	}
	if wErr := guardNotWorking(subtreeIDsOf(chatID, snapshot.rows), u.work); wErr != nil {
		return ChatDeletion{}, wErr
	}
	chats, folders := snapshot.subtree(chatID)
	chats = append(chats, chatID)
	if err := u.purgeAll(ctx, snapshot, chats); err != nil {
		return ChatDeletion{}, err
	}
	if err := u.removeAll(ctx, snapshot, folders); err != nil {
		return ChatDeletion{}, err
	}
	snapshot.plan.Reorder(current.ParentID, "", -1)
	shifted, err := u.persist(ctx, snapshot)
	if err != nil {
		return ChatDeletion{}, err
	}
	return ChatDeletion{Chats: chats, Folders: folders, Shifted: shifted}, nil
}

// purgeAll erases each chat in order and takes it out of the plan as it goes, so
// the densify that follows counts only the rows that survived.
func (u *chatFolderUsecase) purgeAll(
	ctx context.Context,
	snapshot *treeSnapshot,
	ids []string,
) error {
	for _, id := range ids {
		if err := u.agent.PurgeChat(ctx, id); err != nil {
			return fmt.Errorf("agent chat folder: purge chat %s: %w", id, err)
		}
		snapshot.drop(id)
	}
	return nil
}

// removeAll erases each folder caught inside a purged subtree. Forget, not
// PurgeChat: they are removed rather than promoted because the level that would
// have held them is gone, and a folder never had a runner or a ledger for the
// agent usecase to tear down in the first place.
func (u *chatFolderUsecase) removeAll(
	ctx context.Context,
	snapshot *treeSnapshot,
	ids []string,
) error {
	for _, id := range ids {
		if err := u.chats.Forget(ctx, id); err != nil {
			return fmt.Errorf("agent chat folder: delete %s: %w", id, err)
		}
		snapshot.drop(id)
	}
	return nil
}

// replace moves one row to its destination and leaves BOTH affected levels
// dense: the one it joined, and — only when it actually changed level — the one
// it left. Leaving every level dense after every move is what makes the next
// drop index mean what it says.
func (u *chatFolderUsecase) replace(
	snapshot *treeSnapshot,
	id string,
	origin string,
	destination string,
	requested *int,
) {
	target := placementTarget(requested, snapshot, origin, destination, id)
	snapshot.plan.SetParent(id, destination)
	snapshot.plan.Reorder(destination, id, target)
	if destination != origin {
		snapshot.plan.Reorder(origin, "", -1)
	}
}
