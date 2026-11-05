package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// The checks every move passes before anything is written. Folder ops (repo-wide) and chat ops
// (workspace-scoped) carry different container rules, so each gets its own pair.

// checkFolderMove refuses a move onto a missing container, into its own subtree, outside its repo scope, or
// across contexts within that repo. Only a move checks context: a new folder takes on its parent's.
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

// workspaceMove is one workspace placement as its guards see it: the workspace that owns the git lineage,
// the Node row that draws it (an ordinary fork's is its owning chat), and the origin and destination.
type workspaceMove struct {
	repoID      string
	workspaceID string
	nodeID      string
	origin      string
	destination string
}

// checkWorkspaceMove refuses a branch placement onto a missing container, into its own subtree, outside its
// repo, or splitting its fork chain. It skips checkFolderContextMove: a branch is itself a context anchor,
// so comparing its own id with the destination's anchor would refuse every legal move.
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

// checkFolderContextMove refuses a folder crossing context: project, repo root, locked branch, unlocked
// branch. The anchor is walked from live Node positions (Chat.ParentID is frozen once Node-backed) for the
// current and proposed position, and they must match.
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

// nearestWorkspaceAnchor walks id's parent chain, id included, to the first KindWorkspace Node, or "" at
// the root (repo root or project home). A WorkspaceID on a row is not an anchor: every plain chat carries
// one, and stopping there would put a chat and a folder at the same position in different contexts.
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
		// A row with no Node (an owning chat placed by placeOwningRow) is placed by its Chat.ParentID,
		// which is live for it.
		row, err := u.resolveRow(ctx, snapshot, id)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", id, err)
		}
		if row.Type == workspaceAnchorType {
			return id, nil // a live workspace with no Node row yet (resolveRow's last tier)
		}
		id = row.ParentID
	}
	return "", nil
}

// resolveRow answers id's Chat-shaped view for a validation walk, trying in turn the snapshot, Chats, the
// Folders store, a workspace's anchor Node, then a live workspace with no Node yet (a root anchor), since
// the walk does not reach every container a placement can name.
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
	return nil, err // Chats.Get's failure: nothing else answers to id either
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

// checkFolderContainer enforces the scoping rule: a folder's parent ("" always legal) must exist and share
// its repo scope, "" with "" and a repo id only with the same one, so ListInRepo's filter holds.
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

// repoScopeOf answers a row's repo scope: a folder's stored RepoID, a workspace row's repo via RepoOf, or
// nil for a workspace-less bubble, which has nothing to conflict with.
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

// checkChatMove refuses a chat move onto a missing container, another workspace's chat, or the chat's own
// subtree. ownWorktree is true only for createOwnWorktreeChat.
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

// checkChatContainer validates a chat's parent: "" is the root, otherwise the resolved row must pass
// checkParentKind. Another workspace's chat is a cross-workspace refusal, not a missing row.
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

// checkParentKind accepts folders, branches and anchors unconditionally (process boundaries, not workspace
// ones), and a worktree-owning parent for an own-worktree create; an ordinary thread inherits its parent's
// cwd, so a chat parent must be in the same workspace. A repo phantom is never a container.
func checkParentKind(
	row domain.Chat,
	workspaceID string,
	parentID string,
	ownWorktree bool,
) error {
	if row.Type == nodePhantomType {
		return fmt.Errorf("agent chat folder: parent %s: %w", parentID, ErrNotAContainer)
	}
	// A locked branch's anchor, like a branch row, is a process boundary holding many chats, not a
	// workspace boundary.
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
