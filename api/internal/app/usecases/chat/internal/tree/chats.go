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
		ctx, in.WorkspaceID, chatID, PlaceInput{ParentID: &in.ParentID}, placeMode{firstPlacement: true},
	); pErr != nil {
		return CreatedChat{}, u.discard(ctx, chatID, pErr)
	}
	runnerID, err := u.agent.OpenChat(ctx, chatID, in.ProviderID)
	if err != nil {
		return CreatedChat{}, u.discard(ctx, chatID, err)
	}
	return CreatedChat{ChatID: chatID, RunnerID: runnerID}, nil
}

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

func (u *chatFolderUsecase) createImportChat(
	ctx context.Context,
	in CreateChatInput,
) (CreatedChat, error) {
	chatID, ws, runnerID, err := u.createImportedWorktreeChat(ctx, in.ID, in.ProviderID, in.ParentID, in.Worktree.Import, true)
	if err != nil && chatID != "" {
		return CreatedChat{}, u.discard(ctx, chatID, err)
	}
	if err != nil {
		return CreatedChat{}, err
	}
	return CreatedChat{ChatID: chatID, RunnerID: runnerID, WorkspaceID: ws.ID}, nil
}

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
			ctx, "", chatID, PlaceInput{ParentID: &in.ParentID}, placeMode{ownWorktree: true, firstPlacement: true},
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
	if in.WorkspaceID != "" && in.WorkspaceID != workspaceID {
		return u.requestMove(ctx, workspaceID, chatID, in)
	}
	return u.placeChat(ctx, workspaceID, chatID, in, placeMode{})
}

type placeMode struct {
	ownWorktree, firstPlacement, rebind bool
}

func (u *chatFolderUsecase) placeChat(
	ctx context.Context,
	workspaceID string,
	chatID string,
	in PlaceInput,
	mode placeMode,
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
	snapshot, err := u.workspaceSnapshotAround(ctx, workspaceID, current, destination)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	nodeBacked := mode.ownWorktree && destination != ""
	if nodeBacked {
		snapshot.homeIDs[chatID] = true
		snapshot.freshIDs[chatID] = true
	}
	if mErr := u.checkChatMove(ctx, snapshot, workspaceID, chatID, destination, mode.ownWorktree); mErr != nil {
		return domain.Chat{}, nil, mErr
	}
	if fErr := u.checkOwnedForkChain(ctx, snapshot, current, destination); fErr != nil {
		return domain.Chat{}, nil, fErr
	}
	if !mode.rebind {
		if wErr := guardNotWorking(ctx, snapshot.subtreeIDs(chatID), u.busy); wErr != nil {
			return domain.Chat{}, nil, wErr
		}
	}
	inherited := snapshot.chatLineage(chatID)
	u.replace(snapshot, chatID, current.ParentID, destination, in.Order, mode.firstPlacement)
	written, err := u.persist(ctx, snapshot)
	if err != nil {
		return domain.Chat{}, nil, err
	}
	placed := *snapshot.placedRow(chatID)
	if nodeBacked {
		if _, err := u.chats.SetPlacement(ctx, chatID, placed.ParentID, placed.Order); err != nil {
			return domain.Chat{}, nil, fmt.Errorf("agent chat folder: place %s: %w", chatID, err)
		}
	}
	if !mode.rebind {
		u.noteNewAncestors(ctx, chatID, inherited, snapshot.chatLineage(chatID))
	}
	return placed, written, nil
}

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
		row.ParentID = destination
		snapshot.restated[id] = true
	}
}
