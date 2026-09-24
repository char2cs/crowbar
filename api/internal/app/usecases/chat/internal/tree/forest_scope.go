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

// scopeForWorkspace resolves the scope a workspace's rows sit in: home for
// a project-home workspace, a bubble's for "", and for any repo workspace
// the REPO ROOT — the one level the bare root stands for in that tree, whose
// own chats are the default checkout's. A branch's inner level is its
// anchor's container, never the root.
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
	return u.scopeForFolder(ctx, domain.Folder{RepoID: repoID}), nil
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
	aliases levelAliases,
	row *domain.Chat,
) bool {
	switch n.Kind {
	case domain.NodeKindChat:
		return row != nil && u.chatAtRoot(ctx, *row, scope, aliases)
	case domain.NodeKindFolder:
		return u.folderAtRoot(ctx, n.ID, scope)
	case domain.NodeKindRepo:
		return scope.home && (scope.repoMemberIDs == nil || scope.repoMemberIDs[n.ID])
	case domain.NodeKindWorkspace:
		// The level's own default checkout is its header, never a member.
		if scope.repoID == "" || n.ID == scope.workspaceID {
			return false
		}
		repoID, err := u.workspaces.RepoOf(ctx, n.ID)
		return err == nil && repoID == scope.repoID
	}
	return false
}

// folderAtRoot is rootMember's folder case: a repo folder of the scope's
// repo, or a home folder of its home — adopting a legacy one on the way.
func (u *chatFolderUsecase) folderAtRoot(
	ctx context.Context,
	id string,
	scope forestScope,
) bool {
	f, err := u.folders.FindByKey(ctx, id)
	if err != nil || f == nil {
		return false
	}
	if scope.repoID != "" {
		return f.RepoID == scope.repoID
	}
	if f.RepoID == "" && f.HomeID == "" && scope.homeID != "" {
		*f = u.adoptHomeFolder(ctx, *f, scope.homeID)
	}
	return f.InHome(scope.homeID)
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
	aliases levelAliases,
) map[string]bool {
	foreign := map[string]bool{}
	for _, row := range rows {
		if aliases.canonical(row.ParentID) == "" && row.Type != nodePhantomType &&
			row.Type != workspaceAnchorType && row.Type != domain.ChatTypeFolder &&
			!u.chatAtRoot(ctx, row, scope, aliases) {
			foreign[row.ID] = true
		}
	}
	return foreign
}

// chatAtRoot answers whether a chat sitting at the root level belongs to
// scope's level: the scope's own workspace's chats, or a fork's own row (the
// chat that owns a workspace of the scope's repo). A chat that owns a level
// the sidebar draws as something else — the header's, a locked branch's, the
// home's — takes no slot anywhere; its level is the one it names. A bubble
// owns no workspace to name a level by and is a member of the bubble scope
// alone.
func (u *chatFolderUsecase) chatAtRoot(
	ctx context.Context,
	row domain.Chat,
	scope forestScope,
	aliases levelAliases,
) bool {
	if row.WorkspaceID == "" {
		return true
	}
	if scope.zero() {
		return false
	}
	if aliases.canonical(row.ID) != row.ID {
		return false
	}
	if row.WorkspaceID == scope.workspaceID {
		return true
	}
	if aliases.canonical(row.WorkspaceID) != row.ID && scope.workspaceID != "" {
		return false
	}
	repoID, err := u.workspaces.RepoOf(ctx, row.WorkspaceID)
	return err == nil && repoID == scope.repoID
}

// zero reports the bubble scope: no level at all.
func (s forestScope) zero() bool {
	return !s.home && s.repoID == "" && s.workspaceID == ""
}

// levelAliases maps every id a row can be filed under to the id of the row
// the sidebar draws that level as. A workspace's own anchor Node and the
// chat that owns it name ONE level: the header's (the default checkout) and
// the home's is the root, ""; a locked branch's is its anchor; an ordinary
// fork's is its chat. Rows keep whichever of the two ids they were filed
// under; only the plan counts them together.
type levelAliases map[string]string

func (a levelAliases) canonical(
	id string,
) string {
	if c, ok := a[id]; ok {
		return c
	}
	return id
}

// levelAliasesOf resolves the alias set over rows: every workspace some row
// names, with the chat that RECORDS owning it (or a legacy branch row) —
// never a heuristic winner, which could be the very thread being placed.
func (u *chatFolderUsecase) levelAliasesOf(
	ctx context.Context,
	rows []domain.Chat,
) levelAliases {
	holders := map[string][]domain.Chat{}
	for _, row := range rows {
		if row.WorkspaceID != "" && row.Type != workspaceAnchorType && row.OwnsWorkspace {
			holders[row.WorkspaceID] = append(holders[row.WorkspaceID], row)
		}
	}
	aliases := levelAliases{}
	defaults := map[string]string{}
	for wsID, group := range holders {
		ownerID := ""
		if owner, ok := domain.ResolveOwningChat(group); ok {
			ownerID = owner.ID
		}
		u.aliasLevel(ctx, aliases, defaults, wsID, ownerID)
	}
	return aliases
}

// aliasLevel records the level wsID and its owner ownerID name together.
func (u *chatFolderUsecase) aliasLevel(
	ctx context.Context,
	aliases levelAliases,
	defaults map[string]string,
	wsID string,
	ownerID string,
) {
	repoID, err := u.workspaces.RepoOf(ctx, wsID)
	if err != nil {
		return
	}
	if repoID != "" {
		if _, seen := defaults[repoID]; !seen {
			defaults[repoID], _ = u.workspaces.DefaultWorkspaceOf(ctx, repoID)
		}
	}
	switch {
	case repoID == "" || defaults[repoID] == wsID:
		aliases[wsID] = ""
		if ownerID != "" {
			aliases[ownerID] = ""
		}
	case ownerID == "":
	default:
		if renders, err := u.workspaces.RendersAsBranch(ctx, wsID); err == nil && renders {
			aliases[ownerID] = wsID
		} else {
			aliases[wsID] = ownerID
		}
	}
}
