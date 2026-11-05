package tree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The chat half of the tree: placing a chat, and the cascade that takes its
// threads with it when it goes.

func (u *chatFolderUsecase) CreateChat(
	ctx context.Context,
	in CreateChatInput,
) (CreatedChat, error) {
	switch in.Worktree.Mode {
	case WorktreeFork:
		return u.createOwnWorktreeChat(ctx, in)
	case WorktreeImport:
		return u.createImportChat(ctx, in)
	case WorktreeNone:
	}
	if in.WorkspaceID == "" && in.ParentID == "" {
		return CreatedChat{}, ErrNowhereToRun
	}
	if err := u.checkWorkspaceProvisioned(ctx, in.WorkspaceID); err != nil {
		return CreatedChat{}, err
	}
	if err := u.checkThreadParent(ctx, in.WorkspaceID, in.ParentID); err != nil {
		return CreatedChat{}, err
	}
	chatID, err := u.agent.MintChat(ctx, in.ID, in.WorkspaceID, in.ProviderID, in.Surface)
	if err != nil {
		return CreatedChat{}, fmt.Errorf("agent chat folder: create chat: %w", err)
	}
	if _, _, pErr := u.placeChat(
		ctx, in.WorkspaceID, chatID, PlaceInput{ParentID: &in.ParentID}, false, true,
	); pErr != nil {
		return CreatedChat{}, u.discard(ctx, chatID, pErr)
	}
	runnerID, err := u.agent.OpenChat(ctx, chatID, in.ProviderID)
	if err != nil {
		return CreatedChat{}, u.discard(ctx, chatID, err)
	}
	return CreatedChat{ChatID: chatID, RunnerID: runnerID}, nil
}

// checkThreadParent is checkNewChatParent for a plain create. Under a chat that
// runs in a workspace, a create naming none left that workspace out: it is told
// to name it rather than that it crossed one.
func (u *chatFolderUsecase) checkThreadParent(ctx context.Context, workspaceID, parentID string) error {
	if parentID == "" {
		return nil
	}
	err := u.checkNewChatParent(ctx, workspaceID, parentID, false)
	if workspaceID == "" && errors.Is(err, ErrCrossWorkspace) {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrThreadNeedsWorkspace)
	}
	return err
}

// createImportChat is CreateChat's WorktreeImport branch: a single create the
// user was told failed leaves no chat behind.
func (u *chatFolderUsecase) createImportChat(
	ctx context.Context,
	in CreateChatInput,
) (CreatedChat, error) {
	chatID, ws, runnerID, err := u.createImportedWorktreeChat(ctx, in.ID, in.ProviderID, in.ParentID, in.Worktree.Import)
	if err != nil && chatID != "" {
		return CreatedChat{}, u.discard(ctx, chatID, err)
	}
	if err != nil {
		return CreatedChat{}, err
	}
	return CreatedChat{ChatID: chatID, RunnerID: runnerID, WorkspaceID: ws.ID}, nil
}

// createOwnWorktreeChat is CreateChat's WorktreeFork branch: it mints and places the chat in the
// workspace-less scope (a bubble until its worktree exists; ownWorktree allows a worktree-owning chat
// parent), then forks its worktree. Placement comes first so the fork parent is fixed.
func (u *chatFolderUsecase) createOwnWorktreeChat(
	ctx context.Context,
	in CreateChatInput,
) (CreatedChat, error) {
	if in.ProviderID == "" {
		return CreatedChat{}, ErrForkNeedsProvider
	}
	if in.ParentID != "" {
		if err := u.checkNewChatParent(ctx, "", in.ParentID, true); err != nil {
			return CreatedChat{}, err
		}
	}
	chatID, err := u.agent.MintChat(ctx, in.ID, "", in.ProviderID, in.Surface)
	if err != nil {
		return CreatedChat{}, fmt.Errorf("agent chat folder: create chat: %w", err)
	}
	if in.ParentID != "" {
		if _, _, pErr := u.placeChat(
			ctx, "", chatID, PlaceInput{ParentID: &in.ParentID}, true, true,
		); pErr != nil {
			return CreatedChat{}, u.discard(ctx, chatID, pErr)
		}
	}
	wsID, runnerID, err := u.agent.SpawnChatWithOwnWorktree(ctx, chatID, in.ProviderID, in.Worktree.Branch)
	if err != nil {
		return CreatedChat{}, u.discard(ctx, chatID, err)
	}
	return CreatedChat{ChatID: chatID, RunnerID: runnerID, WorkspaceID: wsID}, nil
}

// checkWorkspaceProvisioned refuses a create into a placeholder workspace before anything is minted (see
// ErrWorkspaceUnprovisioned). A failed read and the workspace-less scope pass: it refuses only what it knows.
func (u *chatFolderUsecase) checkWorkspaceProvisioned(
	ctx context.Context,
	workspaceID string,
) error {
	if workspaceID == "" {
		return nil
	}
	provisioned, err := u.workspaces.Provisioned(ctx, workspaceID)
	if err != nil || provisioned {
		return nil
	}
	return fmt.Errorf("%w (workspace %s)", ErrWorkspaceUnprovisioned, workspaceID)
}

// checkNewChatParent refuses a destination a new chat cannot be born into before anything is minted, so a
// mistyped id costs no create-broadcast-purge. It is a move's guard minus the cycle test; ownWorktree lets
// a fork name a worktree-owning chat parent the same-workspace rule would refuse.
func (u *chatFolderUsecase) checkNewChatParent(
	ctx context.Context,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if err := u.ensureWorkspaceAnchor(ctx, parentID); err != nil {
		return err
	}
	snapshot, err := u.workspaceSnapshot(ctx, workspaceID)
	if err != nil {
		return err
	}
	return u.checkChatContainer(ctx, snapshot, workspaceID, parentID, ownWorktree)
}

// ensureWorkspaceAnchor mints the KindWorkspace Node a live workspace from before Node rows lacks, the first
// time a row is filed under it: container checks and fork walks resolve a workspace id through Node.
func (u *chatFolderUsecase) ensureWorkspaceAnchor(
	ctx context.Context,
	id string,
) error {
	if id == "" {
		return nil
	}
	if _, err := u.nodes.GetNode(ctx, id); err == nil {
		return nil
	}
	if _, err := u.chats.Get(ctx, id); err == nil {
		return nil
	}
	if f, err := u.folders.FindByKey(ctx, id); err == nil && f != nil {
		return nil
	}
	if live, err := u.workspaces.Exists(ctx, id); err != nil || !live {
		return nil // not a workspace either: the container check reports it
	}
	if _, err := u.nodes.Create(ctx, id, dtree.KindWorkspace, "", 0); err != nil {
		return fmt.Errorf("agent chat folder: parent %s: mint workspace node: %w", id, err)
	}
	return nil
}

// discard purges a just-minted chat whose create then failed and returns the cause. The purge is
// best-effort and never replaces the cause; a leftover placed, CLI-less chat is visible and deletable.
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
	return u.placeChat(ctx, workspaceID, chatID, in, false, false)
}

// placeChat is PlaceChat's body. ownWorktree is set only by createOwnWorktreeChat, whose destination is
// re-checked here and needs the same allowance; firstPlacement, set by creates, skips densifying the
// root a fresh mint was never drawn in (see replace).
func (u *chatFolderUsecase) placeChat(
	ctx context.Context,
	workspaceID string,
	chatID string,
	in PlaceInput,
	ownWorktree bool,
	firstPlacement bool,
) (domain.Chat, []domain.Chat, error) {
	current, err := u.loadChat(ctx, workspaceID, chatID)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	destination := current.ParentID
	if in.ParentID != nil {
		destination = *in.ParentID
	}
	if err := u.ensureWorkspaceAnchor(ctx, destination); err != nil {
		return domain.Chat{}, nil, err
	}
	// The destination is seeded so its existing children are counted.
	snapshot, err := u.workspaceSnapshotAround(ctx, workspaceID, current, destination)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	nodeBacked := ownWorktree && destination != ""
	if nodeBacked {
		// A fork reads its position through Node once it owns a workspace
		// (correctHomePlacement), so its first placement is minted there too.
		snapshot.homeIDs[chatID] = true
		snapshot.freshIDs[chatID] = true
	}
	if mErr := u.checkChatMove(ctx, snapshot, workspaceID, chatID, destination, ownWorktree); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if fErr := u.checkOwnedForkChain(ctx, snapshot, current, destination); fErr != nil {
		return domain.Chat{}, nil, fErr
	}
	if wErr := guardNotWorking(ctx, snapshot.subtreeIDs(chatID), u.busy); wErr != nil {
		return domain.Chat{}, nil, wErr
	}
	// Read before the plan is mutated, the only moment the old lineage is recoverable; comparing it with
	// the new one decides whether the move is worth recording.
	inherited := snapshot.chatLineage(chatID)
	u.replace(snapshot, chatID, current.ParentID, destination, in.Order, firstPlacement)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	placed := *snapshot.placedRow(chatID)
	if nodeBacked {
		// The write-once-at-creation copy the fork-parent walk reads.
		if _, err := u.chats.SetPlacement(ctx, chatID, placed.ParentID, placed.Order); err != nil {
			return domain.Chat{}, nil, fmt.Errorf("agent chat folder: place %s: %w", chatID, err)
		}
	}
	u.noteNewAncestors(ctx, chatID, inherited, snapshot.chatLineage(chatID))
	return placed, written, nil
}

// noteNewAncestors records the move in the chat's conversation only when it gained a chat to read: moves
// between folders change nothing, and leaving a parent adds no context. A failure is only logged, since
// the move already stands and the relationship rides on ParentID.
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

// replace moves one row and keeps both affected levels dense (the one it joined, and the one it left when
// it changed level) so the next drop index means what it says. firstPlacement skips the level-left densify:
// a fresh mint was never drawn at the root, and closing that phantom gap would renumber unrelated rows.
func (u *chatFolderUsecase) replace(
	snapshot *treeSnapshot,
	id string,
	origin string,
	destination string,
	requested *int,
	firstPlacement bool,
) {
	target := placementTarget(requested, snapshot, origin, destination, id, firstPlacement)
	level, from := snapshot.canonical(destination), snapshot.canonical(origin)
	snapshot.plan.SetParent(id, level)
	snapshot.plan.Reorder(level, id, target)
	if level != from && !firstPlacement {
		snapshot.plan.Reorder(from, "", -1)
	}
	if row := snapshot.row(id); row != nil && row.ParentID != destination {
		// Filed under the other id of the same level: stored as asked.
		row.ParentID = destination
		snapshot.restated[id] = true
	}
}
