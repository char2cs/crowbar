package purge

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func WorktreeRemover(
	crowbarHome string,
	rows func(ctx context.Context) ([]domain.Workspace, error),
) func(ctx context.Context, tomb domain.Workspace) error {
	return func(ctx context.Context, tomb domain.Workspace) error {
		root, ok, err := ownRoot(ctx, crowbarHome, tomb, rows)
		if err != nil || !ok {
			return err
		}
		if err := removeWorkspaceRoot(root); err != nil {
			return fmt.Errorf("purge: remove workspace root %q: %w", root, err)
		}
		pruneEmptiedWorkspaceParents(root, crowbarHome)
		return nil
	}
}

func ownRoot(
	ctx context.Context,
	crowbarHome string,
	tomb domain.Workspace,
	rows func(ctx context.Context) ([]domain.Workspace, error),
) (string, bool, error) {
	root, ok := worktreepath.OwnRoot(tomb.WorktreePath, crowbarHome)
	if !ok {
		if tomb.WorktreePath != "" {
			slog.WarnContext(ctx, "purge: not a single workspace's root; only git removes its checkout",
				"workspace_id", tomb.ID, "path", tomb.WorktreePath, "home", crowbarHome)
		}
		return "", false, nil
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return "", false, nil
	}
	if !resolvesInPlace(root, crowbarHome) || worktreepath.IsLiveCheckout(root) {
		slog.WarnContext(ctx, "purge: workspace root is a symlink or a checkout; kept",
			"workspace_id", tomb.ID, "root", root)
		return "", false, nil
	}
	all, err := rows(ctx)
	if err != nil {
		return "", false, fmt.Errorf("purge: list workspaces: %w", err)
	}
	for _, other := range all {
		if other.ID == tomb.ID {
			continue
		}
		for _, claim := range claimsOf(other, crowbarHome) {
			if overlaps(root, claim) {
				slog.WarnContext(ctx, "purge: workspace root overlaps another workspace; kept",
					"workspace_id", tomb.ID, "root", root, "other", other.ID, "claim", claim)
				return "", false, nil
			}
		}
	}
	return root, true, nil
}

func claimsOf(
	ws domain.Workspace,
	crowbarHome string,
) []string {
	if ws.WorktreePath == "" {
		return nil
	}
	if root, ok := worktreepath.OwnRoot(ws.WorktreePath, crowbarHome); ok {
		return []string{root}
	}
	path := filepath.Clean(ws.WorktreePath)
	return []string{path, worktreepath.ChatsDir(path)}
}

func overlaps(a, b string) bool {
	return a == b || worktreepath.UnderHome(a, b) || worktreepath.UnderHome(b, a)
}

func resolvesInPlace(
	root string,
	crowbarHome string,
) bool {
	rel, err := filepath.Rel(crowbarHome, root)
	if err != nil {
		return false
	}
	resolvedHome, err := filepath.EvalSymlinks(crowbarHome)
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(root)
	return err == nil && resolved == filepath.Join(resolvedHome, rel)
}

var workspaceRootOwned = []string{"worktree", "chats", "storages", "threads", ".DS_Store"}

func removeWorkspaceRoot(
	root string,
) error {
	for _, name := range workspaceRootOwned {
		entry := filepath.Join(root, name)
		if name == "worktree" && worktreepath.IsLiveCheckout(entry) {
			slog.Warn("purge: keeping a worktree git still has registered", "worktree", entry)
			continue
		}
		if err := os.RemoveAll(entry); err != nil {
			return err
		}
	}
	err := os.Remove(root)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	rest, readErr := os.ReadDir(root)
	if readErr != nil {
		return err
	}
	kept := make([]string, 0, len(rest))
	for _, e := range rest {
		kept = append(kept, e.Name())
	}
	slog.Warn("purge: workspace root kept; it holds entries crowbar did not create",
		"root", root, "kept", kept)
	return nil
}

func pruneEmptiedWorkspaceParents(
	root string,
	crowbarHome string,
) {
	floor, ok := projectDirOf(root, crowbarHome)
	if !ok {
		return
	}
	for dir := filepath.Dir(root); worktreepath.UnderHome(dir, floor); dir = filepath.Dir(dir) {
		if err := os.Remove(dir); err != nil {
			return
		}
	}
}

func projectDirOf(
	path string,
	crowbarHome string,
) (string, bool) {
	rel, err := filepath.Rel(crowbarHome, path)
	if err != nil {
		return "", false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "projects" || parts[1] == "" || parts[1] == ".." {
		return "", false
	}
	return filepath.Join(crowbarHome, parts[0], parts[1]), true
}
