// Package layout converts workspaces from the name-keyed on-disk layout to the
// identity-keyed one, once, at boot.
//
//	OLD  <home>/projects/<projectID>/<slug>/<branch>/worktree   (real directory)
//	NEW  <home>/projects/<projectID>/workspaces/<wsID>/worktree (real directory)
//	     <home>/projects/<projectID>/<slug>/<branch>            (symlink to it)
//
// It exists so the rest of the daemon knows exactly ONE layout. Every place that
// resolves, renames or removes a workspace would otherwise need a branch for
// "…or it might still be at its old name-derived path", and those branches are
// what the old code was made of.
//
// The symlink makes it safe against a home full of live workspaces: every
// absolute path already recorded — terminal session cwds, review threads, agent
// runner rows, git's own worktree registration — keeps resolving through it, so
// nothing outside ~/.crowbar is touched and no other store has to be rewritten.
//
// This package is temporary. Once no home predates the identity-keyed layout it
// can be deleted outright: nothing else imports it, and Run degrades to a list
// scan that finds no candidates.
package layout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// GitRepairer re-points git's worktree registration at a moved worktree.
type GitRepairer interface {
	WorktreeRepair(ctx context.Context, repoPath string, worktreePath string) error
}

// Relocator records the new path on the workspace aggregate and its id→path
// index. Without it the move would be invisible to everything that reads the
// record, which after this change is everything.
type Relocator interface {
	Relocate(ctx context.Context, id string, worktreePath string) error
}

// Workspace is the subset of a workspace record the migration needs.
type Workspace struct {
	ID           string
	ProjectID    string
	WorktreePath string
	// RepoPath is the repository the worktree belongs to, for git repair. Empty
	// when it cannot be resolved; the move still happens and the alias keeps
	// git working until something repairs it.
	RepoPath string
	// AdoptedPath is set ONLY for a workspace that is an adopted checkout — a
	// repo home (its path IS the repository) or a project home (its path IS the
	// project folder). Crowbar does not own those directories and must never
	// move them.
	//
	// It is a FACT off the repo/project record, not a guess from the path's
	// shape. Deciding adoption by asking "is the worktree under crowbar home"
	// looked equivalent and was not — an adopted checkout can live inside the
	// home — and that mistake moved three real repositories.
	AdoptedPath string
}

// Result reports what a run did, for the boot log.
type Result struct {
	Migrated int
	Skipped  int
	Failed   int
}

// Run moves every managed workspace still living at its name-derived path to its
// identity-keyed root and leaves a symlink behind.
//
// Idempotent and resumable: a workspace already at its identity root is skipped,
// and every step is safe to re-run after an interruption. Failures are
// per-workspace — one wedged tree never stops the others and never fails boot,
// because a daemon that refuses to start is worse than one workspace still
// sitting at its old path with a record that still names it correctly.
func Run(
	ctx context.Context,
	home string,
	workspaces []Workspace,
	git GitRepairer,
	rec Relocator,
) Result {
	var res Result
	for _, ws := range workspaces {
		switch err := migrateOne(ctx, home, ws, git, rec); {
		case errors.Is(err, errSkip):
			res.Skipped++
		case err != nil:
			res.Failed++
			slog.Error("layout migration: workspace not migrated",
				"ws", ws.ID, "path", ws.WorktreePath, "err", err)
		default:
			res.Migrated++
		}
	}
	if res.Migrated > 0 || res.Failed > 0 {
		slog.Info("layout migration complete",
			"migrated", res.Migrated, "skipped", res.Skipped, "failed", res.Failed)
	}
	return res
}

var errSkip = errors.New("layout: nothing to migrate")

func migrateOne(
	ctx context.Context,
	home string,
	ws Workspace,
	git GitRepairer,
	rec Relocator,
) error {
	if ws.ProjectID == "" || ws.ID == "" {
		return errSkip
	}
	if ws.AdoptedPath != "" {
		return healAdopted(ctx, ws, rec)
	}
	if ws.WorktreePath == "" {
		// An unprovisioned placeholder has no tree to move.
		return errSkip
	}
	p, err := planMove(home, ws)
	if err != nil {
		return err
	}
	return applyMove(ctx, home, ws, p, git, rec)
}

// healAdopted keeps an adopted checkout where it is and only corrects a record
// that has drifted off it.
//
// ADOPTED CHECKOUTS ARE NEVER MOVED — but their record can still be wrong, and
// then nothing else can fix it: the path it names does not exist, so every later
// resolve, merge and remove fails on a workspace whose real directory is sitting
// there untouched. The repo/project record knows where that is, so heal it here
// rather than leave a home needing hand surgery.
func healAdopted(ctx context.Context, ws Workspace, rec Relocator) error {
	if ws.WorktreePath == ws.AdoptedPath {
		return errSkip
	}
	if _, err := os.Stat(ws.AdoptedPath); err != nil {
		return errSkip
	}
	slog.Info("layout migration: restoring an adopted checkout's recorded path",
		"ws", ws.ID, "from", ws.WorktreePath, "to", ws.AdoptedPath)
	return rec.Relocate(ctx, ws.ID, ws.AdoptedPath)
}

// movePlan is where a managed workspace's tree is, and where it has to land.
type movePlan struct {
	// oldRoot is the directory that MOVES, and the path the alias replaces.
	oldRoot string
	// dest is where oldRoot lands: the new root for a leaf-shaped workspace,
	// the worktree leaf inside it for a bare one.
	dest string
	// root and worktree are the identity-keyed destinations, whatever the shape.
	root     string
	worktree string
}

// planMove resolves which of the two managed layouts a record is in, and refuses
// anything that is not Crowbar's to move.
func planMove(home string, ws Workspace) (movePlan, error) {
	// TWO managed shapes reach here, and the difference is where the workspace
	// root is relative to the recorded path:
	//
	//	LEAF  <slug>/<branch>/worktree   — the root is the PARENT
	//	BARE  <slug>/<branch>            — the checkout IS the recorded path,
	//	                                   the shape every branch provisioned
	//	                                   before the leaf existed still has
	//
	// Treating the leaf as mandatory left the bare ones behind, and a bare one
	// left behind is a real directory standing exactly where this layout expects
	// a symlink — the one thing LinkAlias and UnlinkAlias both refuse to touch.
	// The workspace opened fine and then could not be renamed or removed.
	//
	// The leaf is therefore not the safety guard; it never was. What keeps a
	// checkout Crowbar does not own from moving is AdoptedPath above and the
	// project-directory test below, and for a bare path the additional demand
	// that it actually BE a checkout — a bare directory with no .git is a
	// container (a branch prefix like <slug>/feature), and filepath.Dir of one
	// of those is how a run once moved a whole tree of repositories.
	bare := filepath.Base(ws.WorktreePath) != "worktree"
	if bare && !isCheckout(ws.WorktreePath) {
		return movePlan{}, errSkip
	}
	// ADOPTED CHECKOUTS ARE NEVER TOUCHED. A repo-home or project-home workspace
	// points at a repository Crowbar does not own, and moving one would relocate
	// the user's own directory out from under them.
	//
	// The test is the MANAGED PROJECT DIRECTORY, not crowbar home. "Under home"
	// looked equivalent and is not: an adopted checkout can sit inside the home
	// and still not be ours — the dev seed keeps them at <home>/seed/<project>,
	// and that guard duly migrated all three, replacing the repositories with
	// symlinks. Only <home>/projects/<projectID>/... is Crowbar's to move.
	projectDir := filepath.Join(home, "projects", ws.ProjectID)
	if !underRoot(ws.WorktreePath, projectDir) {
		return movePlan{}, errSkip
	}
	// The directory that MOVES, and where it lands. A leaf-shaped workspace moves
	// its whole root (worktree plus the chats tree beside it); a bare one is only
	// the checkout, so it moves into the leaf the new root expects.
	p := movePlan{
		oldRoot:  filepath.Dir(ws.WorktreePath),
		root:     filepath.Join(projectDir, "workspaces", ws.ID),
		worktree: filepath.Join(projectDir, "workspaces", ws.ID, "worktree"),
	}
	p.dest = p.root
	if bare {
		p.oldRoot = ws.WorktreePath
		p.dest = p.worktree
	}
	if p.oldRoot == p.root {
		return movePlan{}, errSkip
	}
	return p, nil
}

// applyMove performs the move, publishes the alias and records the new path, in
// the one order that leaves a working workspace after an interruption anywhere.
func applyMove(
	ctx context.Context,
	home string,
	ws Workspace,
	p movePlan,
	git GitRepairer,
	rec Relocator,
) error {
	// RESUME. A run interrupted after the move but before the record write leaves
	// the tree at the new root while the record still names the old path — and
	// the old path is no longer a directory to move, it is the alias pointing at
	// the new one. Keying this off the SOURCE was wrong for exactly that reason:
	// os.Stat follows the symlink, reports the old path as present, and the retry
	// tries to rename a directory onto itself. The destination is what says
	// whether the move already happened.
	if _, err := os.Stat(p.worktree); err == nil {
		// Republish the alias in case the interruption landed before it, then
		// catch the record up.
		if linkErr := linkAlias(p.oldRoot, p.root); linkErr != nil {
			return fmt.Errorf("publish alias: %w", linkErr)
		}
		return rec.Relocate(ctx, ws.ID, p.worktree)
	}
	if _, err := os.Stat(p.oldRoot); errors.Is(err, os.ErrNotExist) {
		return errSkip
	}

	//nolint:gosec // G301: matches the 0755 the rest of the worktree layout uses.
	if err := os.MkdirAll(filepath.Dir(p.dest), 0o755); err != nil {
		return fmt.Errorf("create workspaces dir: %w", err)
	}
	if err := os.Rename(p.oldRoot, p.dest); err != nil {
		return fmt.Errorf("move workspace root: %w", err)
	}
	// Legacy per-workspace storages lived OUTSIDE the root; carry them in so the
	// root really is the whole footprint one rm -rf can take.
	adoptLegacyStorages(home, ws, p.root)
	// The alias goes where the real directory was, so every path recorded against
	// the old location still resolves — including git's registration, which is
	// why the repair below is allowed to fail without stranding anything.
	if err := linkAlias(p.oldRoot, p.root); err != nil {
		return fmt.Errorf("publish alias: %w", err)
	}
	if git != nil && ws.RepoPath != "" {
		if err := git.WorktreeRepair(ctx, ws.RepoPath, p.worktree); err != nil {
			slog.Warn("layout migration: worktree repair failed; the alias keeps git resolving",
				"ws", ws.ID, "repo", ws.RepoPath, "err", err)
		}
	}
	// LAST: until this lands the record still names the old path, which the alias
	// resolves — so an interruption anywhere above leaves a working workspace,
	// and the re-drive above finishes the job.
	if err := rec.Relocate(ctx, ws.ID, p.worktree); err != nil {
		return fmt.Errorf("record new path: %w", err)
	}
	return nil
}

// linkAlias puts a symlink where the real directory used to be.
func linkAlias(alias string, root string) error {
	//nolint:gosec // G301: as above.
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(alias); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("alias path %q is occupied by a real directory", alias)
		}
		if rmErr := os.Remove(alias); rmErr != nil {
			return rmErr
		}
	}
	return os.Symlink(root, alias)
}

// adoptLegacyStorages moves <projectID>/<repoID>/workspaces/<wsID>/* into the
// new root. The repo id is not on the record here, so the directory is found by
// glob; a workspace created after the layout change has none.
func adoptLegacyStorages(home string, ws Workspace, newRoot string) {
	projectDir := filepath.Join(home, "projects", ws.ProjectID)
	matches, err := filepath.Glob(filepath.Join(projectDir, "*", "workspaces", ws.ID))
	if err != nil {
		return
	}
	for _, old := range matches {
		if old == newRoot || strings.HasPrefix(old, newRoot+string(filepath.Separator)) {
			continue
		}
		entries, readErr := os.ReadDir(old)
		if readErr != nil {
			continue
		}
		for _, e := range entries {
			dst := filepath.Join(newRoot, e.Name())
			if _, statErr := os.Stat(dst); statErr == nil {
				continue // the new root already owns this; leave it alone
			}
			if mvErr := os.Rename(filepath.Join(old, e.Name()), dst); mvErr != nil {
				slog.Warn("layout migration: could not adopt legacy storage",
					"ws", ws.ID, "entry", e.Name(), "err", mvErr)
			}
		}
		_ = os.RemoveAll(old)
	}
}

// isCheckout reports whether a bare recorded path is really a git checkout, by
// the presence of `.git` — a FILE holding a gitdir pointer in a linked worktree,
// a directory in a main one.
//
// It is what separates a pre-leaf workspace from a mere branch-prefix directory
// (<slug>/feature, standing over <slug>/feature/x/worktree). Both are bare
// directories under the project; only one is a workspace, and moving the other
// would take every branch beneath it along.
func isCheckout(path string) bool {
	_, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil
}

// underRoot reports whether path is strictly under root, the same
// directory-boundary test every removal guard uses.
func underRoot(path string, root string) bool {
	if path == "" || root == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(root, "/")+"/")
}
