// Package purge holds the hardened on-disk half of a workspace purge: the one
// function allowed to delete a workspace's root under the crowbar home. The
// delete reactor and the boot sweep both reach it through reactors.Purger, so
// the guards below hold on every path.
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

// WorktreeRemover builds the one bounded fs delete a workspace purge uses to
// remove a deleted workspace's on-disk footprint — for the delete reactor and the
// boot sweep alike, through the same reactors.Purger (spec §3 P0-2).
//
// It removes a directory only when it can prove that directory is ONE
// workspace's own root (ownRoot). Every other shape — a pre-leaf
// <slug>/<branch> checkout whose parent every sibling shares, a path outside the
// home, a root another row still claims — loses nothing here: its checkout is
// git's to remove (the delete usecase's teardown), and whatever is left beside
// it is shared or foreign. rows lists every workspace row, tombstones included.
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

// ownRoot answers the directory that belongs to tomb alone, or false. The
// proof has four parts: the path has the managed leaf shape
// (worktreepath.OwnRoot); the root on disk is that very directory, not a symlink
// into somewhere else; it is not itself a checkout; and no other row's claim
// (claimsOf) overlaps it. A root that is already gone is false: nothing to do.
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

// claimsOf lists the directories a row may own: its own root for the leaf
// shape; otherwise its checkout and the chats tree resolved beside it, which
// for a pre-leaf row is shared by every sibling.
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

// overlaps reports whether a and b are the same directory or one holds the
// other.
func overlaps(a, b string) bool {
	return a == b || worktreepath.UnderHome(a, b) || worktreepath.UnderHome(b, a)
}

// resolvesInPlace reports whether root, with every symlink resolved, is still
// root: no component of it leads somewhere else in (or out of) the home.
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

// workspaceRootOwned are the entries a workspace root may lose. The first four
// are the directories Crowbar itself creates; .DS_Store is macOS metadata that
// appears the moment the user opens the root in Finder, and keeping the root
// alive for it would turn every browsed workspace into permanent litter.
var workspaceRootOwned = []string{"worktree", "chats", "storages", "threads", ".DS_Store"}

// removeWorkspaceRoot deletes the workspace's own directories, then the root —
// which succeeds only once nothing else is left in it.
//
// It replaces an rm -rf of the entire root. That was written to the rule "the
// root IS the workspace's whole on-disk footprint", and on a real machine it is
// not: a <slug>/<branch> root was found holding five hand-made git worktrees
// beside the managed one, so deleting that workspace would have taken 4.5GB of
// checkouts Crowbar never created. A foreign entry now keeps the root alive and
// is reported instead of destroyed.
//
// The worktree leaf is the one owned entry it may still refuse: while it holds a
// `.git` link it is a checkout git still has registered — one `git worktree
// remove` declined, typically a protected worktree holding uncommitted work
// (spec §3 P0-1). Git owns removing a registered worktree; an rm -rf would
// both destroy that work and leave a dangling registration in the user's repo.
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

// pruneEmptiedWorkspaceParents removes the directories a workspace-root removal
// emptied, walking up from the root's parent.
//
// It cannot delete anything it did not empty and it cannot climb out of the
// project. os.Remove only ever succeeds on an EMPTY directory, so a slug still
// holding a sibling workspace stops the walk on its own; and the floor is
// <home>/projects/<projectID>, which is never a candidate — that level holds the
// project's icon, its `workspaces` state and its repo directories beside the
// slug trees, so climbing into it would delete live state rather than litter.
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

// projectDirOf returns <home>/projects/<projectID> for a path beneath it, and
// false for anything not laid out that way — an adopted checkout, or a path the
// managed layout does not explain, neither of which may be climbed.
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
