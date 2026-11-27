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

const StepOwnerOnly = 11

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

type ownerWalk struct {
	narrowed         int64
	unreachable      int64
	firstUnreachable string
}

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

type openEntry struct {
	rel  string
	perm fs.FileMode
}

type homeScan struct {
	open      []openEntry
	checkouts []string
}

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

func isCheckout(root *os.Root, rel, name string) bool {
	if name == "worktree" || name == ".git" {
		return true
	}
	_, err := root.Lstat(filepath.Join(rel, ".git"))
	return err == nil
}
