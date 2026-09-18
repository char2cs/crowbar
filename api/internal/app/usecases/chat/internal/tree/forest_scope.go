package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Which level a snapshot's bare root stands for, and who belongs to it — the
// one canonical sibling set every writer (chat placement, folder CRUD,
// workspace placement) counts against, mirroring what the sidebar renders.

// forestScope names the ONE level a snapshot's bare root ("") stands for.
// Node.ParentID "" is shared by every project's home rows, every repo's root
// rows and every workspace anchor, so a walk that took every child of ""
// as a sibling renumbered rows of other levels as collateral and landed the
// caller's index slots off. Under a real container (a folder, a chat, an
// anchor) every child IS a sibling; only the root needs this.
//
// The zero value is a bubble's scope: nothing at the root but the bubbles
// themselves and the home folders they may be filed beside.
type forestScope struct {
	// home is a project-home level: this project's repo headers (repoMemberIDs,
	// nil for "unknown project — every repo") and its home folders (homeID,
	// "" for "unknown home — every home folder").
	home          bool
	homeID        string
	repoMemberIDs map[string]bool
	// repoID is a repo's own root level: that repo's folders and the locked
	// branches it draws, never a repo header or a home row.
	repoID string
	// workspaceID is whose root chats sit at this level: the home workspace,
	// a repo's default checkout, or the locked branch whose own row the
	// level hangs under. "" means "any workspace of this scope".
	workspaceID string
}

// scopeForWorkspace resolves the scope a workspace's own rows sit in: home
// for a project-home workspace, its repo otherwise, a bubble's for "".
func (u *chatFolderUsecase) scopeForWorkspace(
	ctx context.Context,
	workspaceID string,
) (forestScope, error) {
	if workspaceID == "" {
		return forestScope{}, nil
	}
	repoID, err := u.workspaces.RepoOf(ctx, workspaceID)
	if err != nil {
		return forestScope{}, fmt.Errorf("agent chat folder: resolve workspace %s: %w", workspaceID, err)
	}
	if repoID == "" {
		return forestScope{
			home: true, homeID: workspaceID, workspaceID: workspaceID,
			repoMemberIDs: u.repoMemberIDsForHome(ctx, workspaceID),
		}, nil
	}
	return forestScope{repoID: repoID, workspaceID: workspaceID}, nil
}

// scopeForFolder resolves a folder's scope off its own stored identity. A
// repo folder's level is the repo root, whose chats are the default
// checkout's.
func (u *chatFolderUsecase) scopeForFolder(
	ctx context.Context,
	f domain.Folder,
) forestScope {
	if f.RepoID != "" {
		scope := forestScope{repoID: f.RepoID}
		if id, err := u.workspaces.DefaultWorkspaceOf(ctx, f.RepoID); err == nil {
			scope.workspaceID = id
		}
		return scope
	}
	scope := forestScope{home: true, homeID: f.HomeID, workspaceID: f.HomeID}
	if f.HomeID != "" {
		scope.repoMemberIDs = u.repoMemberIDsForHome(ctx, f.HomeID)
	}
	return scope
}

// scopeForSubject resolves the scope for a snapshot built around one row:
// a folder's own stored scope, otherwise the scope of the workspace the row
// belongs to. An unknown subject degrades to an unscoped home root.
func (u *chatFolderUsecase) scopeForSubject(
	ctx context.Context,
	subject domain.Chat,
) (forestScope, error) {
	if subject.ID == "" {
		return forestScope{home: true}, nil
	}
	if subject.Type == domain.ChatTypeFolder {
		if f, err := u.folders.FindByKey(ctx, subject.ID); err == nil && f != nil {
			return u.scopeForFolder(ctx, *f), nil
		}
		return u.scopeForFolder(ctx, domain.Folder{ID: subject.ID, RepoID: subject.RepoID}), nil
	}
	return u.scopeForWorkspace(ctx, subject.WorkspaceID)
}

// scopeForBubble resolves the level a workspace-less chat's root stands for
// off the container it currently hangs in — a bubble owns no workspace to
// name a scope, but the folder or chat above it does. A bubble at the root
// with no such ancestor keeps the zero scope.
func (u *chatFolderUsecase) scopeForBubble(
	ctx context.Context,
	subject domain.Chat,
) forestScope {
	if subject.ParentID == "" {
		return forestScope{}
	}
	if f, err := u.folders.FindByKey(ctx, subject.ParentID); err == nil && f != nil {
		return u.scopeForFolder(ctx, *f)
	}
	if parent, err := u.chats.Get(ctx, subject.ParentID); err == nil && parent.WorkspaceID != "" {
		if scope, sErr := u.scopeForWorkspace(ctx, parent.WorkspaceID); sErr == nil {
			return scope
		}
	}
	return forestScope{}
}

// rootMember decides whether a Node found at the bare root belongs to
// scope's level. Every kind's own membership fact lives on a different
// aggregate, so each is asked separately. row is the base row the Node
// corrects, nil when the walk discovered it.
func (u *chatFolderUsecase) rootMember(
	ctx context.Context,
	n domain.Node,
	scope forestScope,
	row *domain.Chat,
) bool {
	switch n.Kind {
	case domain.NodeKindChat:
		return row != nil && u.chatAtRoot(ctx, *row, scope)
	case domain.NodeKindFolder:
		f, err := u.folders.FindByKey(ctx, n.ID)
		if err != nil || f == nil {
			return false
		}
		if scope.repoID != "" {
			return f.RepoID == scope.repoID
		}
		return f.InHome(scope.homeID)
	case domain.NodeKindRepo:
		return scope.home && (scope.repoMemberIDs == nil || scope.repoMemberIDs[n.ID])
	case domain.NodeKindWorkspace:
		if scope.repoID == "" {
			return false
		}
		repoID, err := u.workspaces.RepoOf(ctx, n.ID)
		return err == nil && repoID == scope.repoID
	}
	return false
}

// foreignAtRoot names the base rows sitting at the bare root that are not
// scope's own: a global read (ListChats) carries every workspace's root chats,
// and only the level being planned may count them as siblings. They stay in
// the snapshot — a walk still resolves through them and their own children
// are still discovered — but off the root level of the plan (see
// treeSnapshot.foreign).
func (u *chatFolderUsecase) foreignAtRoot(
	ctx context.Context,
	rows []domain.Chat,
	scope forestScope,
) map[string]bool {
	foreign := map[string]bool{}
	for _, row := range rows {
		if row.ParentID == "" && row.Type != nodePhantomType && row.Type != workspaceAnchorType &&
			row.Type != domain.ChatTypeFolder && !u.chatAtRoot(ctx, row, scope) {
			foreign[row.ID] = true
		}
	}
	return foreign
}

// chatAtRoot answers whether a chat sitting at the bare root belongs to
// scope's level: the scope's own workspace, or — when the scope names none
// — any workspace of the scope's repo (home). A bubble owns no workspace to
// name a level by and stays a root member wherever it is asked about.
func (u *chatFolderUsecase) chatAtRoot(
	ctx context.Context,
	row domain.Chat,
	scope forestScope,
) bool {
	if row.WorkspaceID == "" {
		return true
	}
	if scope.workspaceID != "" {
		return row.WorkspaceID == scope.workspaceID
	}
	repoID, err := u.workspaces.RepoOf(ctx, row.WorkspaceID)
	return err == nil && repoID == scope.repoID
}

// withoutHomeOwner drops the chat that owns the home workspace itself: it is
// the workspace's own addressable row, not a row the top level draws, so it
// takes no slot there.
func withoutHomeOwner(
	rows []domain.Chat,
) []domain.Chat {
	owner, ok := domain.ResolveOwningChat(rows)
	if !ok {
		return rows
	}
	out := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		if row.ID != owner.ID {
			out = append(out, row)
		}
	}
	return out
}

// isHomeWorkspace answers whether workspaceID is a project's home workspace —
// RepoOf resolving "" is the SAME convention repoScopeOf/checkFolderContainer
// already use elsewhere in this package. A bubble ("") is never home: it is
// mid-creation, ownerless (see workspaceSnapshotAround's own doc), and never
// resolves through RepoOf at all.
func (u *chatFolderUsecase) isHomeWorkspace(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	if workspaceID == "" {
		return false, nil
	}
	repoID, err := u.workspaces.RepoOf(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("agent chat folder: resolve workspace %s: %w", workspaceID, err)
	}
	return repoID == "", nil
}
