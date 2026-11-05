// Package holder classifies who holds a git branch across a repo's worktrees, so import
// and the worktree usecase can tell free, managed, or held-elsewhere (freed only with
// user consent). Resolution never detaches: it only prunes dead registrations.
package holder

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// Kind classifies who holds a branch.
type Kind int

const (
	// Free: no worktree holds the branch (after pruning dead registrations).
	Free Kind = iota
	// HeldByHome: the repo's main folder (the unmanaged default workspace).
	HeldByHome
	// HeldByManaged: a Crowbar-managed worktree under <crowbarHome> — already a
	// workspace; never double-provision.
	HeldByManaged
	// HeldByExternal: a live worktree the user made outside the crowbar home.
	HeldByExternal
)

// Outcome is a resolved holder classification. HeldByPath is the holder's
// worktree directory (empty for Free).
type Outcome struct {
	Kind       Kind
	HeldByPath string
}

// Engine is the narrow git surface Resolve needs. DetachWorktree is deliberately
// absent: detaching is a separate, consented operation.
type Engine interface {
	WorktreePrune(ctx context.Context, repoPath string) error
	WorktreeList(ctx context.Context, repoPath string) ([]gitengine.WorktreeEntry, error)
}

// Resolve prunes dead registrations, then classifies the worktree holding branch.
// A prune failure only leaves a possibly-stale list; a WorktreeList failure is returned.
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

// isUnder reports whether path is at or below root (symlink-resolved).
func isUnder(path, root string) bool {
	if root == "" {
		return false
	}
	rp := worktreepath.ResolvePath(path)
	rr := worktreepath.ResolvePath(root)
	return rp == rr || strings.HasPrefix(rp, rr+string(filepath.Separator))
}
