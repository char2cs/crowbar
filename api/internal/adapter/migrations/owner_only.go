package migrations

import (
	"context"
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
	n, err := ownerOnly(env.Home.Root(), false)
	if err != nil || n == 0 {
		return migrate.Plan{}, err
	}
	return migrate.Plan{Actions: []migrate.Action{{
		Kind: migrate.Convert, Subject: "files and directories others can read → owner-only", Count: n,
	}}}, nil
}

func narrowOwnerOnly(_ context.Context, env migrate.Env, _ *migrate.Checkpoints) (migrate.Counts, error) {
	n, err := ownerOnly(env.Home.Root(), true)
	if err != nil {
		return nil, err
	}
	return migrate.Counts{"narrowed": n}, nil
}

// ownerOnly counts, and with apply narrows, every regular file and directory
// under root that grants group or other bits, never entering a checkout.
func ownerOnly(root string, apply bool) (int64, error) {
	var n int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == "worktree" || d.Name() == ".git"):
			return fs.SkipDir
		case !d.IsDir() && !d.Type().IsRegular():
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		perm := info.Mode().Perm()
		if perm&0o077 == 0 {
			return nil
		}
		n++
		if !apply {
			return nil
		}
		return os.Chmod(path, perm&^0o077)
	})
	if err != nil {
		return 0, fmt.Errorf("owner-only: %w", err)
	}
	return n, nil
}
