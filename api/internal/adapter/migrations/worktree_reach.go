package migrations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

// StepWorktreeReach gives back traverse, for group and others, on the way to
// each checkout, which an earlier build's owner-only step took away, so another
// uid reaches a worktree through the home again.
const StepWorktreeReach = 13

func planWorktreeReach(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	dirs, err := unreachableWay(env.Home.Root())
	if err != nil || len(dirs) == 0 {
		return migrate.Plan{}, err
	}
	return migrate.Plan{Actions: []migrate.Action{{
		Kind: migrate.Convert, Subject: "directories on the way to a checkout others cannot traverse → traverse",
		Count: int64(len(dirs)),
	}}}, nil
}

func reachWorktrees(_ context.Context, env migrate.Env, _ *migrate.Checkpoints) (migrate.Counts, error) {
	home := env.Home.Root()
	dirs, err := unreachableWay(home)
	if err != nil {
		return nil, err
	}
	checkouts, err := homeCheckouts(home)
	if err != nil {
		return nil, err
	}
	for _, c := range checkouts {
		if err := worktreepath.Reach(home, filepath.Join(home, c)); err != nil {
			return nil, fmt.Errorf("worktree reach: %w", err)
		}
	}
	return migrate.Counts{"reachable": int64(len(dirs))}, nil
}

// unreachableWay names, once each, the directories on the way to a checkout
// that others cannot traverse.
func unreachableWay(home string) ([]string, error) {
	checkouts, err := homeCheckouts(home)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range checkouts {
		dirs, err := worktreepath.Unreachable(home, filepath.Join(home, c))
		if err != nil {
			return nil, fmt.Errorf("worktree reach: %w", err)
		}
		for _, d := range dirs {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out, nil
}

// homeCheckouts lists the checkouts under projects/, relative to home.
func homeCheckouts(home string) (_ []string, err error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, fmt.Errorf("worktree reach: %w", err)
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	scan, err := scanHome(root, &ownerWalk{})
	return scan.checkouts, err
}
