// Package purge holds the hardened on-disk half of a workspace purge: the one
// function allowed to delete a workspace's root under the crowbar home. The
// delete reactor and the boot sweep both reach it through reactors.Purger, so
// the guards below (under the home, no foreign entries, no checkout git
// still registers) hold on every path.
package purge

import (
	"fmt"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// WorktreeRemover builds the one bounded fs delete a workspace purge uses to
// remove a deleted workspace's on-disk footprint — for the delete reactor and the
// boot sweep alike, through the same reactors.Purger (spec §3 P0-2).
//
// path is the tombstone's WorktreePath: the "worktree" leaf of a workspace root
// that also holds the sibling "chats" tree. `git worktree remove` only clears
// the leaf, so this removes the ROOT's Crowbar-made entries and then the root.
//
// It is GUARDED: only a path strictly under the crowbar home is ever touched —
// an adopted home or main worktree's path is the user's REAL checkout, outside
// the home, and is never deleted. A blank path, a path outside the home, or an already-gone dir is
// an idempotent no-op, so a crash re-driven purge rm's to nothing.
func WorktreeRemover(
	crowbarHome string,
) func(path string) error {
	return func(path string) error {
		if !worktreepath.UnderHome(path, crowbarHome) {
			if path != "" {
				slog.Warn("purge: refusing to rm worktree outside the crowbar home",
					"path", path, "home", crowbarHome)
			}
			return nil
		}
		// The removed target is the PARENT of the worktree leaf (the workspace
		// root holding the sibling chats tree). Re-guard the ROOT itself: a
		// degenerate one-segment leaf (<home>/worktree) has filepath.Dir == home,
		// and rm'ing that would nuke the ENTIRE crowbar home. Only a root that is
		// still STRICTLY under home — i.e. path had an intermediate segment below
		// home — is ever removed.
		root := filepath.Dir(path)
		if !worktreepath.UnderHome(root, crowbarHome) {
			slog.Warn("purge: refusing to rm workspace root at or above the crowbar home",
				"root", root, "path", path, "home", crowbarHome)
			return nil
		}
		if err := removeWorkspaceRoot(root); err != nil {
			return fmt.Errorf("purge: remove workspace root %q: %w", root, err)
		}
		pruneEmptiedWorkspaceParents(root, crowbarHome)
		return nil
	}
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
