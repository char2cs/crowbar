package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// StepOwnerOnly narrows what an older build created with the umask under the
// home to owner-only, as this build creates it. A user's checkout is git's.
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

// ownerOnly counts, and with apply narrows, every regular file and directory
// under home that grants group or other bits, never entering a checkout. It
// works through an os.Root so a symlink cannot lead it outside the home.
func ownerOnly(home string, apply bool) (w ownerWalk, err error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return w, fmt.Errorf("owner-only: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return w.skip(rel, err)
		case d.IsDir() && (d.Name() == "worktree" || d.Name() == ".git"):
			return fs.SkipDir
		case !d.IsDir() && !d.Type().IsRegular():
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return w.skip(rel, err)
		}
		perm := info.Mode().Perm()
		if perm&0o077 == 0 {
			return nil
		}
		if !apply {
			w.narrowed++
			return nil
		}
		if err := root.Chmod(rel, perm&^0o077); err != nil {
			return w.skip(rel, err)
		}
		w.narrowed++
		return nil
	})
	if err != nil {
		return ownerWalk{}, fmt.Errorf("owner-only: %w", err)
	}
	return w, nil
}
