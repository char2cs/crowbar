package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Which repo a row belongs to.
//
// No chat row carries a repo id, and none should: a row's repo is a fact about
// the ground under it, which is the workspace its cwd walk lands on (model spec
// §3.2). Storing it as well would be the second field §5's "derive, do not
// store" rejects — one that goes stale the moment a row is dragged.

// ListChatsInRepo returns every conversation-typed row whose cwd walk resolves
// to a workspace in repoID — the answer GET /repos/:rid/chats owes, and the one
// it could not give while a repo-scoped list fell back to every chat the daemon
// knows.
//
// Folder rows are dropped: the panel reads those through its own repo-scoped
// folder list. So is a true ORPHAN — a row whose whole ancestry owns no
// workspace — because a row that belongs to no repo cannot belong to this one,
// and returning it would put a chat nothing can resolve into every repo's list
// at once.
func (u *Usecase) ListChatsInRepo(
	ctx context.Context,
	repoID string,
) ([]domain.Chat, error) {
	rows, err := u.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: list chats in repo %s: %w", repoID, err)
	}
	cwds := tree.CwdWorkspaceIDs(rows)
	resolved := map[string]string{}
	out := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		if u.rowInRepo(ctx, row, cwds, resolved, repoID) {
			out = append(out, row)
		}
	}
	return out, nil
}

// rowInRepo answers whether one row belongs to repoID, reusing resolved as the
// per-call memo of what each workspace's repo turned out to be. One list can
// hold many rows over one workspace, and the answer cannot change mid-call.
func (u *Usecase) rowInRepo(
	ctx context.Context,
	row domain.Chat,
	cwds map[string]string,
	resolved map[string]string,
	repoID string,
) bool {
	if row.Type == domain.ChatTypeFolder {
		return false
	}
	workspaceID, ok := cwds[row.ID]
	if !ok {
		return false
	}
	return u.repoOfWorkspace(ctx, workspaceID, resolved) == repoID
}

// repoOfWorkspace resolves one workspace's owning repo, memoized into resolved.
//
// A workspace that cannot be resolved at all answers the empty repo, which is
// never what a repo-scoped route asks for, so an unresolvable row is excluded
// rather than served into whichever repo happened to ask. The failure is not
// propagated: one broken row must not fail the whole list.
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
// ancestor-or-self carrying one (model spec §3.2). It is the same walk the
// spawn path resolves a bubble's cwd through, exposed for the callers OUTSIDE
// this feature that need a row's ground rather than its stored slot — above
// all the WS fan-out, which has to namespace a bubble's frames under the repo
// the row actually runs in.
func (u *Usecase) CwdWorkspaceID(
	ctx context.Context,
	chatID string,
) (string, bool, error) {
	return tree.ResolveCwdWorkspaceID(ctx, u.chats, chatID)
}
