package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// StepOwnerOnly narrows what an older build created with the umask under the
// home to owner-only, as this build creates it. A user's checkout is git's, and
// the directories on the way to one are left as they are.
const StepOwnerOnly = 11

// planOwnerOnly counts the files and directories others can still reach.
func planOwnerOnly(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	walk, err := ownerOnly(env.Home.Root(), false)
	if err != nil || walk.narrowed == 0 {
		return migrate.Plan{}, err
	}
	return migrate.Plan{Actions: []migrate.Action{{
		Kind: migrate.Convert, Subject: "files and directories others can read → owner-only", Count: walk.narrowed,
	}}}, nil
}

func narrowOwnerOnly(ctx context.Context, env migrate.Env, _ *migrate.Checkpoints) (migrate.Counts, error) {
	walk, err := ownerOnly(env.Home.Root(), true)
	if err != nil {
		return nil, err
	}
	counts := migrate.Counts{"narrowed": walk.narrowed}
	if walk.unreachable > 0 {
		env.Log.WarnContext(ctx, "owner-only: left entries Crowbar may not read or change as they are",
			"count", walk.unreachable, "first", filepath.Join(env.Home.Root(), walk.firstUnreachable))
		counts["unreachable"] = walk.unreachable
	}
	return counts, nil
}

// ownerWalk is what one pass over the home found.
type ownerWalk struct {
	narrowed         int64
	unreachable      int64
	firstUnreachable string
}

// skip counts an entry Crowbar may not read or chmod, such as one another user
// owns: narrowing is hardening, and it must not keep the daemon from starting.
func (w *ownerWalk) skip(rel string, err error) error {
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	if w.unreachable == 0 {
		w.firstUnreachable = rel
	}
	w.unreachable++
	return nil
}

// openEntry is a file or directory that grants group or other bits.
type openEntry struct {
	rel  string
	perm fs.FileMode
}

// homeScan is one pass over the home: what grants group or other bits, and
// the checkouts under projects/, which are git's and never entered.
type homeScan struct {
	open      []openEntry
	checkouts []string
}

// ownerOnly counts, and with apply narrows, every regular file and directory
// under home that grants group or other bits, except a checkout and the
// directories on the way to one, which another uid may need to traverse.
func ownerOnly(home string, apply bool) (w ownerWalk, err error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return w, fmt.Errorf("owner-only: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	scan, err := scanHome(root, &w)
	if err != nil {
		return ownerWalk{}, err
	}
	way := map[string]bool{}
	for _, c := range scan.checkouts {
		for dir := filepath.Dir(c); !way[dir]; dir = filepath.Dir(dir) {
			way[dir] = true
		}
	}
	for _, e := range scan.open {
		if way[e.rel] {
			continue
		}
		narrowed, err := w.narrow(root, e, apply)
		if err != nil {
			return ownerWalk{}, err
		}
		if narrowed {
			w.narrowed++
		}
	}
	return w, nil
}

// narrow drops e's group and other bits when apply, reporting whether it is
// (or would be) owner-only; one Crowbar may not change is skipped.
func (w *ownerWalk) narrow(root *os.Root, e openEntry, apply bool) (bool, error) {
	if !apply {
		return true, nil
	}
	err := root.Chmod(e.rel, e.perm&^0o077)
	if err == nil {
		return true, nil
	}
	if err := w.skip(e.rel, err); err != nil {
		return false, fmt.Errorf("owner-only: %w", err)
	}
	return false, nil
}

// scanHome walks the home through root, so a symlink cannot lead it outside.
// A checkout is a directory named worktree or holding .git; one under
// projects/ is listed, any is skipped.
func scanHome(root *os.Root, w *ownerWalk) (homeScan, error) {
	var scan homeScan
	err := fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return w.skip(rel, err)
		case d.IsDir() && isCheckout(root, rel, d.Name()):
			if strings.HasPrefix(rel, "projects"+string(filepath.Separator)) {
				scan.checkouts = append(scan.checkouts, rel)
			}
			return fs.SkipDir
		case !d.IsDir() && !d.Type().IsRegular():
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return w.skip(rel, err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			scan.open = append(scan.open, openEntry{rel: rel, perm: perm})
		}
		return nil
	})
	if err != nil {
		return homeScan{}, fmt.Errorf("owner-only: %w", err)
	}
	return scan, nil
}

// isCheckout reports whether the directory rel is a user's checkout.
func isCheckout(root *os.Root, rel, name string) bool {
	if name == "worktree" || name == ".git" {
		return true
	}
	_, err := root.Lstat(filepath.Join(rel, ".git"))
	return err == nil
}
