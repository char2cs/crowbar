package tree

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type forestScope struct {
	home          bool
	homeID        string
	repoMemberIDs map[string]bool
	repoID        string
	workspaceID   string
}

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
		if scope.repoID == "" || n.ID == scope.workspaceID {
			return false
		}
		repoID, err := u.workspaces.RepoOf(ctx, n.ID)
		return err == nil && repoID == scope.repoID
	}
	return false
}

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

func (s forestScope) zero() bool {
	return !s.home && s.repoID == "" && s.workspaceID == ""
}

type levelAliases map[string]string

func (a levelAliases) canonical(
	id string,
) string {
	if c, ok := a[id]; ok {
		return c
	}
	return id
}

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
