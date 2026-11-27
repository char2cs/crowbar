package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func (u *chatFolderUsecase) checkFolderMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	folderRepoID string,
	id string,
	destination string,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkFolderContainer(ctx, snapshot, folderRepoID, destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(snapshot.canonical(destination), id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	if err := u.checkFolderContextMove(ctx, snapshot, id, destination); err != nil {
		return err
	}
	return nil
}

type workspaceMove struct {
	repoID      string
	workspaceID string
	nodeID      string
	origin      string
	destination string
}

func (u *chatFolderUsecase) checkWorkspaceMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	move workspaceMove,
) error {
	if move.destination == move.nodeID {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", move.nodeID, ErrCycle)
	}
	if err := u.checkFolderContainer(ctx, snapshot, move.repoID, move.destination); err != nil {
		return err
	}
	if snapshot.plan.Reaches(snapshot.canonical(move.destination), move.nodeID) {
		return fmt.Errorf(
			"agent chat folder: move %s under %s: %w", move.nodeID, move.destination, ErrCycle,
		)
	}
	return u.checkForkChainSplit(ctx, snapshot, move)
}

func (u *chatFolderUsecase) checkFolderContextMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
	destination string,
) error {
	from, err := u.nearestWorkspaceAnchor(ctx, snapshot, id)
	if err != nil {
		return fmt.Errorf("agent chat folder: move %s: %w", id, err)
	}
	to, err := u.nearestWorkspaceAnchor(ctx, snapshot, destination)
	if err != nil {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, err)
	}
	if from != to {
		return fmt.Errorf(
			"agent chat folder: move %s under %s crosses context: %w",
			id, destination, ErrCrossContext,
		)
	}
	return nil
}

func (u *chatFolderUsecase) nearestWorkspaceAnchor(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (string, error) {
	seen := map[string]bool{}
	for id != "" {
		if seen[id] {
			return "", nil
		}
		seen[id] = true
		n, nErr := u.nodes.GetNode(ctx, id)
		if nErr == nil && n.Kind == dtree.KindWorkspace {
			return id, nil
		}
		if nErr == nil {
			id = n.ParentID
			continue
		}
		row, err := u.resolveRow(ctx, snapshot, id)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", id, err)
		}
		if row.Type == workspaceAnchorType {
			return id, nil
		}
		id = row.ParentID
	}
	return "", nil
}

func (u *chatFolderUsecase) resolveRow(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (*domain.Chat, error) {
	if row := snapshot.row(id); row != nil {
		return row, nil
	}
	got, err := u.chats.Get(ctx, id)
	if err == nil {
		return &got, nil
	}
	if f, ferr := u.folders.FindByKey(ctx, id); ferr == nil && f != nil {
		n, nerr := u.nodes.GetNode(ctx, id)
		if nerr != nil {
			n = dtree.Node{}
		}
		row := homeFolderView(*f, n)
		return &row, nil
	}
	if n, nerr := u.nodes.GetNode(ctx, id); nerr == nil && n.Kind == dtree.KindWorkspace {
		return u.anchorRow(ctx, id, n)
	}
	if live, lerr := u.workspaces.Exists(ctx, id); lerr == nil && live {
		return u.anchorRow(ctx, id, dtree.Node{ID: id, Kind: dtree.KindWorkspace})
	}
	return nil, err
}

func (u *chatFolderUsecase) anchorRow(
	ctx context.Context,
	id string,
	n dtree.Node,
) (*domain.Chat, error) {
	row, err := u.anchorView(ctx, id, n)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (u *chatFolderUsecase) checkFolderContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	folderRepoID string,
	parentID string,
) error {
	if parentID == "" {
		return nil
	}
	row, err := u.resolveRow(ctx, snapshot, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	if row.Type == nodePhantomType {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrNotAContainer)
	}
	parentRepoID, err := u.repoScopeOf(ctx, *row)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	if parentRepoID != nil && *parentRepoID != folderRepoID {
		return fmt.Errorf(
			"agent chat folder: parent %s belongs to repo %q, not %q: %w",
			parentID, *parentRepoID, folderRepoID, ErrCrossContext,
		)
	}
	return nil
}

func (u *chatFolderUsecase) repoScopeOf(ctx context.Context, row domain.Chat) (*string, error) {
	if row.Type == domain.ChatTypeFolder {
		id := row.RepoID
		return &id, nil
	}
	if row.WorkspaceID == "" {
		return nil, nil
	}
	id, err := u.workspaces.RepoOf(ctx, row.WorkspaceID)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (u *chatFolderUsecase) checkChatMove(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	id string,
	destination string,
	ownWorktree bool,
) error {
	if destination == id {
		return fmt.Errorf("agent chat folder: move %s onto itself: %w", id, ErrCycle)
	}
	if err := u.checkChatContainer(ctx, snapshot, workspaceID, destination, ownWorktree); err != nil {
		return err
	}
	if snapshot.plan.Reaches(snapshot.canonical(destination), id) {
		return fmt.Errorf("agent chat folder: move %s under %s: %w", id, destination, ErrCycle)
	}
	return nil
}

func (u *chatFolderUsecase) checkChatContainer(
	ctx context.Context,
	snapshot *treeSnapshot,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if parentID == "" {
		return nil
	}
	row, err := u.resolveRow(ctx, snapshot, parentID)
	if err != nil {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, err)
	}
	return checkParentKind(*row, workspaceID, parentID, ownWorktree)
}

func checkParentKind(
	row domain.Chat,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if row.Type == nodePhantomType {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrNotAContainer)
	}
	if row.Type == domain.ChatTypeFolder || row.Type == domain.ChatTypeBranch || row.Type == workspaceAnchorType {
		return nil
	}
	if ownWorktree && row.WorkspaceID != "" {
		return nil
	}
	if row.WorkspaceID == workspaceID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: parent %s belongs to workspace %s, not %s: %w",
		parentID, row.WorkspaceID, workspaceID, ErrCrossWorkspace,
	)
}
