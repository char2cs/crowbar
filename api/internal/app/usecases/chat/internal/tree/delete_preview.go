package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DeletePreview walks chatID's subtree exactly as DeleteChat and Delete's
// cascading successor are about to, and reports what that walk would take
// without taking it: how many CHAT rows and how many uncommitted files.
//
// The subtree is read GLOBALLY, not scoped to one workspace: chatID may name a
// FOLDER, and a folder's subtree can hold chats from several independent
// workspaces (see WorkspaceGitStatus). Reading only one workspace's rows would
// silently drop every other workspace's files from the count.
//
// chatID may also name a home-scoped folder (2026-09-08
// sidebar-placement-unification Task 5), which is no longer a Chat row at
// all — checked first, the same way Move/Delete/Rename dispatch, before
// falling through to the unchanged Chats.LoadChat path every other id (a
// repo-scoped folder, or any chat) still resolves through.
func (u *chatFolderUsecase) DeletePreview(
	ctx context.Context,
	chatID string,
) (int, int, error) {
	current, err := u.loadForPreview(ctx, chatID)
	if err != nil {
		return 0, 0, fmt.Errorf("agent chat folder: delete preview %s: %w", chatID, err)
	}
	snapshot, err := u.globalSnapshotAround(ctx, current)
	if err != nil {
		return 0, 0, fmt.Errorf("agent chat folder: delete preview %s: %w", chatID, err)
	}
	return u.countSubtree(ctx, snapshot, chatID)
}

// loadForPreview resolves id to its Chat-shaped view for DeletePreview: a
// home-scoped folder (Folder+Node) or, for everything else (a repo-scoped
// folder, or any chat), the unchanged Chats.LoadChat read.
func (u *chatFolderUsecase) loadForPreview(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	f, ferr := u.folders.FindByKey(ctx, id)
	if ferr != nil {
		return domain.Chat{}, ferr
	}
	if f != nil && f.RepoID == "" {
		n, nerr := u.nodes.GetNode(ctx, id)
		if nerr != nil {
			return domain.Chat{}, nerr
		}
		return homeFolderView(*f, n), nil
	}
	return u.chats.LoadChat(ctx, id)
}

// countSubtree sums the two answers DeletePreview needs across id's subtree
// (id included): every CHAT-typed row, and the working-tree file count summed
// across every row that owns a workspace of its own.
func (u *chatFolderUsecase) countSubtree(
	ctx context.Context,
	snapshot *treeSnapshot,
	id string,
) (int, int, error) {
	chatCount := 0
	fileCount := 0
	for _, memberID := range subtreeIDsOf(id, snapshot.rows) {
		files, err := u.tallyMember(ctx, snapshot, memberID)
		if err != nil {
			return 0, 0, fmt.Errorf("agent chat folder: delete preview %s: %w", id, err)
		}
		if snapshot.isChat(memberID) {
			chatCount++
		}
		fileCount += files
	}
	return chatCount, fileCount, nil
}

// tallyMember answers one subtree member's own file-count contribution: zero
// for a row the snapshot no longer carries or one that owns no workspace, and
// the workspace's already-synced Added+Deleted otherwise.
func (u *chatFolderUsecase) tallyMember(
	ctx context.Context,
	snapshot *treeSnapshot,
	memberID string,
) (int, error) {
	row := snapshot.row(memberID)
	if row == nil || row.WorkspaceID == "" {
		return 0, nil
	}
	added, deleted, err := u.workspaces.WorkingTreeSummary(ctx, row.WorkspaceID)
	if err != nil {
		return 0, fmt.Errorf("git status for %s: %w", row.WorkspaceID, err)
	}
	return added + deleted, nil
}
