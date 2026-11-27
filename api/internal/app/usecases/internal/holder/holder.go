package holder

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

type Kind int

const (
	Free Kind = iota
	HeldByHome
	HeldByManaged
	HeldByExternal
)

type Outcome struct {
	Kind       Kind
	HeldByPath string
}

type Engine interface {
	WorktreePrune(ctx context.Context, repoPath string) error
	WorktreeList(ctx context.Context, repoPath string) ([]gitengine.WorktreeEntry, error)
}

func Resolve(
	ctx context.Context,
	git Engine,
	repoPath string,
	branch string,
	crowbarHome string,
) (Outcome, error) {
	if err := git.WorktreePrune(ctx, repoPath); err != nil {
		slog.WarnContext(ctx, "holder: prune dead worktree registrations (classifying the unpruned list)",
			"repo", repoPath, "err", err)
	}
	entries, err := git.WorktreeList(ctx, repoPath)
	if err != nil {
		return Outcome{}, err
	}
	for _, e := range entries {
		if e.Branch != branch {
			continue
		}
		switch {
		case worktreepath.SamePath(e.Path, repoPath):
			return Outcome{Kind: HeldByHome, HeldByPath: e.Path}, nil
		case isUnder(e.Path, crowbarHome):
			return Outcome{Kind: HeldByManaged, HeldByPath: e.Path}, nil
		default:
			return Outcome{Kind: HeldByExternal, HeldByPath: e.Path}, nil
		}
	}
	return Outcome{Kind: Free}, nil
}

func isUnder(path, root string) bool {
	if root == "" {
		return false
	}
	rp := worktreepath.ResolvePath(path)
	rr := worktreepath.ResolvePath(root)
	return rp == rr || strings.HasPrefix(rp, rr+string(filepath.Separator))
}
