package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/core/readonce"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A row's repo is derived, never stored: it is the repo of the workspace its cwd walk
// lands on, and a stored copy would go stale the moment the row is dragged.

// ListChatsInRepo returns every conversation-typed row whose cwd walk resolves to a
// workspace in repoID. Folders are dropped (the panel lists them separately), and so
// is an orphan whose ancestry owns no workspace, which belongs to no repo.
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

// chatsByRepo files every chat under the repo it runs in; one read answers
// every repo a sidebar snapshot lists.
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

// repoOfRow answers the repo one row runs in, memoizing each workspace's repo
// in resolved for the call. A folder, or a row with no workspace to run in, is
// in no repo.
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

// repoOfWorkspace resolves one workspace's owning repo, memoized into resolved. An
// unresolvable workspace answers "", excluding its rows without failing the list.
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

// CwdWorkspaceID answers where a row's CLI runs: the workspace of the nearest
// ancestor-or-self carrying one — the spawn path's walk, exposed for callers like
// the WS fan-out that must namespace a bubble's frames under its real repo.
func (u *Usecase) CwdWorkspaceID(
	ctx context.Context,
	chatID string,
) (string, bool, error) {
	return tree.ResolveCwdWorkspaceID(ctx, u.chats, u.folders, u.nodes, u.repoRoots, chatID)
}
