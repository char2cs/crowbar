package hierarchy

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// teardown is what deleting one workspace does to git. It is decided once, by
// teardownOf, so the loss assessment and the removal can never disagree.
type teardown struct {
	// worktree is the checkout to remove, or "" when it is not Crowbar's.
	worktree string
	force    bool
	// dropBranch is the branch to delete, or "" when it stays.
	dropBranch string
	// reattach puts the main folder back on the default branch the workspace held.
	reattach bool
}

// teardownOf decides a workspace's teardown. Only a worktree Crowbar created is
// ever removed, and it is forced only when unlocked and holding no other row's
// files; only a branch Crowbar created, unlocked and not the default, is deleted.
func (u *hierarchyUsecase) teardownOf(
	ws domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) teardown {
	// No repo to run git against, or a placeholder whose branch is held elsewhere.
	if repo.path == "" || ws.Provisioning == domain.WorkspacePlaceholder {
		return teardown{}
	}
	locked := ws.Status == domain.WorkspaceStatusLocked
	var td teardown
	if u.crowbarWorktree(ws, repo.path) {
		td.worktree = ws.WorktreePath
		td.force = !locked && !worktreepath.HoldsAnother(ws.WorktreePath, otherPaths(ws.ID, all))
	}
	switch {
	case ws.Branch == "":
	case ws.Branch == repo.defaultBranch:
		td.reattach = true
	case ws.CreatedBranch && !locked:
		td.dropBranch = ws.Branch
	}
	return td
}

// crowbarWorktree reports whether ws's checkout is one Crowbar made: recorded
// as provisioned, under <home>/projects, and not the repo's own checkout. The
// main working tree and any checkout adopted in place are the user's.
func (u *hierarchyUsecase) crowbarWorktree(
	ws domain.Workspace,
	repoPath string,
) bool {
	path := ws.WorktreePath
	if ws.Provisioning != domain.WorkspaceProvisioned || path == "" ||
		worktreepath.SamePath(path, repoPath) {
		return false
	}
	home, err := u.crowbarHome()
	if err != nil || home == "" {
		return false
	}
	root := filepath.Join(home, "projects")
	return worktreepath.UnderHome(filepath.Clean(path), root) ||
		worktreepath.UnderHome(worktreepath.ResolvePath(path), worktreepath.ResolvePath(root))
}

// workAtRisk measures what td would destroy: uncommitted paths in a worktree it
// force-removes, and commits reachable only from that worktree's HEAD or from
// the branch it deletes. Stashes are shared by the whole repo and survive.
func (u *hierarchyUsecase) workAtRisk(
	ctx context.Context,
	ws domain.Workspace,
	repo repoRef,
	td teardown,
) (domain.WorkAtRisk, error) {
	risk := domain.WorkAtRisk{WorkspaceID: ws.ID, Branch: ws.Branch}
	live := td.worktree != "" && worktreepath.IsLiveCheckout(td.worktree)
	if live && td.force {
		n, err := u.git.UncommittedFiles(ctx, td.worktree)
		if err != nil {
			return risk, fmt.Errorf("work at risk: %s: uncommitted: %w", ws.ID, err)
		}
		risk.UncommittedFiles = n
	}
	dir, tips := repo.path, []string(nil)
	if live {
		dir, tips = td.worktree, append(tips, "HEAD")
	}
	if td.dropBranch != "" {
		tips = append(tips, "refs/heads/"+td.dropBranch)
	}
	if len(tips) == 0 {
		return risk, nil
	}
	n, err := u.git.UnmergedCommits(ctx, dir, tips, td.dropBranch)
	if err != nil {
		return risk, fmt.Errorf("work at risk: %s: unmerged commits: %w", ws.ID, err)
	}
	risk.UnmergedCommits = n
	return risk, nil
}

// refuseLoss is the one rule every delete passes through: without consent, a
// delete that would destroy work existing nowhere else stops before touching
// anything and names that work. An assessment git cannot make also stops it.
func (u *hierarchyUsecase) refuseLoss(
	ctx context.Context,
	consent domain.DeleteConsent,
	doomed []domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) error {
	if consent == domain.DiscardWorkAtRisk {
		return nil
	}
	risks, err := u.assess(ctx, doomed, repo, all)
	if err != nil {
		return err
	}
	if len(risks) > 0 {
		return domain.WorkAtRisked(risks)
	}
	return nil
}

// assess lists the work each doomed workspace's teardown would destroy.
func (u *hierarchyUsecase) assess(
	ctx context.Context,
	doomed []domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) ([]domain.WorkAtRisk, error) {
	var risks []domain.WorkAtRisk
	for _, ws := range doomed {
		risk, err := u.workAtRisk(ctx, ws, repo, u.teardownOf(ws, repo, all))
		if err != nil {
			return nil, err
		}
		if risk.Lost() {
			risks = append(risks, risk)
		}
	}
	return risks, nil
}

// repoRef is what a workspace teardown needs of its repository: the main
// checkout git runs against, and the default branch that must never be
// deleted. A zero path means the repo could not be resolved.
type repoRef struct {
	path          string
	defaultBranch string
}

// repoRefFor resolves a workspace's repository for its teardown. A missing
// or unreadable row resolves to a zero repoRef — the teardown then drops the
// row without touching git, so a cascade never strands a ghost workspace.
func (u *hierarchyUsecase) repoRefFor(
	ctx context.Context,
	repoID string,
) repoRef {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil || repo == nil {
		slog.WarnContext(ctx, "cascade: repo unresolved; dropping rows without git teardown",
			"repo", repoID, "err", err)
		return repoRef{}
	}
	return repoRef{path: repo.Path, defaultBranch: repo.DefaultBranch}
}

// removeOne tears one workspace down and tombstones its row. Git failures are logged and the row is
// still dropped, so the cascade never leaves a ghost pointing at a gone worktree.
func (u *hierarchyUsecase) removeOne(
	ctx context.Context,
	ws domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) error {
	// Before the worktree goes: a live shell would otherwise outlive its CWD.
	u.reapTerminals(ctx, ws.ID)
	td := u.teardownOf(ws, repo, all)
	if td.worktree == "" && ws.WorktreePath != "" && repo.path != "" {
		slog.InfoContext(ctx, "cascade: not a worktree Crowbar created; leaving it in place",
			"ws", ws.ID, "worktree", ws.WorktreePath)
	}
	if td.worktree != "" {
		if removeErr := u.git.WorktreeRemove(ctx, repo.path, td.worktree, td.force); removeErr != nil {
			slog.WarnContext(ctx, "cascade: worktree remove failed (continuing)",
				"ws", ws.ID, "worktree", td.worktree, "force", td.force, "err", removeErr)
		}
	}
	if td.reattach {
		u.reattachMainIfDetachedAt(ctx, repo.path, ws.Branch)
	}
	if td.dropBranch != "" {
		if delErr := u.git.ForceDeleteBranch(ctx, repo.path, td.dropBranch); delErr != nil {
			slog.WarnContext(ctx, "cascade: branch delete failed (continuing)",
				"ws", ws.ID, "branch", td.dropBranch, "err", delErr)
		}
	}
	return u.workspaces.Delete(ctx, ws.ID)
}

// otherPaths lists the worktree paths of every row but id, tombstones
// included: a tombstone's files may still be on disk.
func otherPaths(
	id string,
	all []domain.Workspace,
) []string {
	paths := make([]string, 0, len(all))
	for _, ws := range all {
		if ws.ID != id {
			paths = append(paths, ws.WorktreePath)
		}
	}
	return paths
}

// reattachMainIfDetachedAt puts the main folder back on branch only when it is still a detached HEAD at
// the branch's tip, as a create's detach left it; any other state is the user's. Best-effort.
func (u *hierarchyUsecase) reattachMainIfDetachedAt(
	ctx context.Context,
	repoPath string,
	branch string,
) {
	entries, err := u.git.WorktreeList(ctx, repoPath)
	if err != nil || len(entries) == 0 || entries[0].Branch != "" {
		return // unreadable, or the main folder is on a branch of its own
	}
	tip, err := u.git.RevParse(ctx, repoPath, "refs/heads/"+branch)
	if err != nil || tip != entries[0].Head {
		return
	}
	if reErr := u.git.CheckoutBranch(ctx, repoPath, branch); reErr != nil {
		slog.WarnContext(ctx, "cascade: re-attach main folder to default branch (continuing)",
			"branch", branch, "err", reErr)
	}
}

// reapTerminals kills every live PTY session in wsID's worktree by fanning out over its chats, since
// sessions are keyed by owning chat. A missing reaper or chat observer skips the reap rather than reap
// under a wrong key.
func (u *hierarchyUsecase) reapTerminals(
	ctx context.Context,
	wsID string,
) {
	if u.terminals == nil || u.chatObserver == nil {
		return
	}
	chats, err := u.chatObserver.ListChatsByWorkspace(ctx, wsID)
	if err != nil {
		slog.WarnContext(ctx, "cascade: terminal reap could not list chats (continuing)",
			"ws", wsID, "err", err)
		return
	}
	for _, chat := range chats {
		for _, sid := range u.terminals.ListSessionsForChat(chat.ID) {
			if killErr := u.terminals.Kill(ctx, sid); killErr != nil {
				slog.WarnContext(ctx, "cascade: terminal kill failed (continuing)",
					"ws", wsID, "chat", chat.ID, "session", sid, "err", killErr)
			}
		}
	}
}
