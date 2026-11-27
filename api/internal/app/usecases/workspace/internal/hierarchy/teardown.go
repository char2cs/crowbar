package hierarchy

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type teardown struct {
	worktree   string
	force      bool
	dropBranch string
	reattach   bool
}

func (u *hierarchyUsecase) teardownOf(
	ws domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) teardown {
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

type repoRef struct {
	path          string
	defaultBranch string
}

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

func (u *hierarchyUsecase) removeOne(
	ctx context.Context,
	ws domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) error {
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

func (u *hierarchyUsecase) reattachMainIfDetachedAt(
	ctx context.Context,
	repoPath string,
	branch string,
) {
	entries, err := u.git.WorktreeList(ctx, repoPath)
	if err != nil || len(entries) == 0 || entries[0].Branch != "" {
		return
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
