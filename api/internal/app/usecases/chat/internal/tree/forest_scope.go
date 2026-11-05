package tree

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Which level a snapshot's bare root stands for, and who belongs to it: the one sibling set every writer
// counts against, mirroring what the sidebar renders.

// forestScope names the level a snapshot's bare root stands for. Node.ParentID "" is shared by every
// project's home, every repo root and every anchor, so only the root needs scoping; under a real container
// every child is a sibling. The zero value is the bubble scope: bubbles and the home folders beside them.
type forestScope struct {
	// home is a project-home level: this project's repo headers (repoMemberIDs, nil for every repo) and
	// its home folders (homeID, "" for every home).
	home          bool
	homeID        string
	repoMemberIDs map[string]bool
	// repoID is a repo's own root level: that repo's folders and the locked
	// branches it draws, never a repo header or a home row.
	repoID string
	// workspaceID is whose root chats sit at this level (home, a repo's default checkout, or a locked
	// branch); "" means any workspace of the scope.
	workspaceID string
}

// scopeForWorkspace resolves the scope a workspace's rows sit in: home for a project-home workspace, the
// bubble scope for "", and the repo root for any repo workspace (a branch's inner level is not the root).
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
	return u.scopeForFolder(ctx, dtree.Folder{RepoID: repoID}), nil
}

// scopeForFolder resolves a folder's scope off its own stored identity. A
// repo folder's level is the repo root, whose chats are the default
// checkout's.
func (u *chatFolderUsecase) scopeForFolder(
	ctx context.Context,
	f dtree.Folder,
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
		return u.scopeForFolder(ctx, dtree.Folder{ID: subject.ID, RepoID: subject.RepoID}), nil
	}
	return u.scopeForWorkspace(ctx, subject.WorkspaceID)
}

// scopeForBubble resolves a workspace-less chat's scope from the folder or chat it hangs in; with no such
// parent it keeps the zero scope.
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

// rootMember decides whether a Node at the bare root belongs to scope's level, asking each kind's own
// aggregate. row is the base row the Node corrects, nil when the walk discovered it.
func (u *chatFolderUsecase) rootMember(
	ctx context.Context,
	n dtree.Node,
	scope forestScope,
	aliases levelAliases,
	row *domain.Chat,
) bool {
	switch n.Kind {
	case dtree.KindChat:
		return row != nil && u.chatAtRoot(ctx, *row, scope, aliases)
	case dtree.KindFolder:
		return u.folderAtRoot(ctx, n.ID, scope)
	case dtree.KindRepo:
		return scope.home && (scope.repoMemberIDs == nil || scope.repoMemberIDs[n.ID])
	case dtree.KindWorkspace:
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

// foreignAtRoot names the base rows at the bare root that are not scope's own: a global read carries every
// workspace's root chats. They stay in the snapshot for walks, but off the plan's root level.
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

// chatAtRoot answers whether a root chat belongs to scope's level: the scope workspace's chats, or a fork's
// owning chat in the scope's repo. A chat whose level the sidebar draws as something else takes no slot;
// a bubble belongs to the bubble scope alone.
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

// levelAliases maps every id a row can be filed under to the id the sidebar draws that level as: a
// workspace's anchor and its owning chat name one level (the root for home and default checkout, the
// anchor for a locked branch, the chat for an ordinary fork). Rows keep the id they were filed under.
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
		u.rememberDefault(ctx, defaults, repoID)
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

// rememberDefault memoises repoID's default checkout in defaults. A lookup that
// fails is logged and remembered as none, which is what a repo without one has.
func (u *chatFolderUsecase) rememberDefault(
	ctx context.Context,
	defaults map[string]string,
	repoID string,
) {
	if _, seen := defaults[repoID]; seen {
		return
	}
	id, err := u.workspaces.DefaultWorkspaceOf(ctx, repoID)
	if err != nil {
		slog.WarnContext(ctx, "agent chat folder: default checkout of a repo", "repo_id", repoID, "err", err)
	}
	defaults[repoID] = id
}
