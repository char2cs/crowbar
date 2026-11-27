package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/core/readonce"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *Usecase) ListChatsInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	byRepo, err := readonce.Do(ctx, "agent:chats-by-repo", func() (map[string][]domain.Chat, error) {
		return u.chatsByRepo(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("agent: list chats in repo %s: %w", repoID, err)
	}
	return append([]domain.Chat{}, byRepo[repoID]...), nil
}

func (u *Usecase) chatsByRepo(ctx context.Context) (map[string][]domain.Chat, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, err
	}
	cwds := tree.CwdWorkspaceIDs(ctx, u.folders, u.nodes, u.repoRoots, rows)
	resolved := map[string]string{}
	out := map[string][]domain.Chat{}
	for _, row := range rows {
		if repoID, ok := u.repoOfRow(ctx, row, cwds, resolved); ok {
			out[repoID] = append(out[repoID], row)
		}
	}
	return out, nil
}

func (u *Usecase) repoOfRow(
	ctx context.Context,
	row domain.Chat,
	cwds map[string]string,
	resolved map[string]string,
) (string, bool) {
	if row.Type == domain.ChatTypeFolder {
		return "", false
	}
	workspaceID, ok := cwds[row.ID]
	if !ok {
		return "", false
	}
	return u.repoOfWorkspace(ctx, workspaceID, resolved), true
}

func (u *Usecase) repoOfWorkspace(
	ctx context.Context,
	workspaceID string,
	resolved map[string]string,
) string {
	if repoID, ok := resolved[workspaceID]; ok {
		return repoID
	}
	_, _, repoID, _, err := u.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		repoID = ""
	}
	resolved[workspaceID] = repoID
	return repoID
}

func (u *Usecase) CwdWorkspaceID(
	ctx context.Context,
	chatID string,
) (string, bool, error) {
	return tree.ResolveCwdWorkspaceID(ctx, u.chats, u.folders, u.nodes, u.repoRoots, chatID)
}
