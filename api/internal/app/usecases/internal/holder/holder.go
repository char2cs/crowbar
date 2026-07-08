// Package holder resolves who currently holds a git branch across a repo's
// worktrees, so both the project-import path and the worktree usecase can decide
// whether a protected branch is free to materialise, already managed, or held by
// a live worktree that must be freed with user consent (spec §3.1). Resolution
// never detaches — it only prunes dead registrations and classifies the holder.
package holder

import (
	"context"
	"path/filepath"
	"strings"

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

// Engine is the narrow git surface Resolve needs — satisfied by both the import
// usecase's ImportGitEngine (once WorktreePrune is added) and the worktree
// usecase's full enginegit.Engine. DetachWorktree is deliberately absent:
// resolution only prunes + lists; the detach is a separate consented op.
type Engine interface {
	WorktreePrune(ctx context.Context, repoPath string) error
	WorktreeList(ctx context.Context, repoPath string) ([]gitengine.WorktreeEntry, error)
}

// Resolve prunes dead-directory registrations, then finds the worktree holding
// branch and classifies it. Pruning is best-effort (it only reaps dead regs, so
// a failure just means classification runs against a possibly-stale list); a
// WorktreeList failure is fatal and returned.
func Resolve(
	ctx context.Context,
	git Engine,
	repoPath string,
	branch string,
	crowbarHome string,
) (Outcome, error) {
	_ = git.WorktreePrune(ctx, repoPath)
	entries, err := git.WorktreeList(ctx, repoPath)
	if err != nil {
		return Outcome{}, err
	}
	for _, e := range entries {
		if e.Branch != branch {
			continue
		}
		switch {
		case samePath(e.Path, repoPath):
			return Outcome{Kind: HeldByHome, HeldByPath: e.Path}, nil
		case isUnder(e.Path, crowbarHome):
			return Outcome{Kind: HeldByManaged, HeldByPath: e.Path}, nil
		default:
			return Outcome{Kind: HeldByExternal, HeldByPath: e.Path}, nil
		}
	}
	return Outcome{Kind: Free}, nil
}

// samePath reports whether two paths refer to the same location, resolving
// symlinks first (git worktree list emits fully-resolved paths, e.g. macOS
// /var -> /private/var), matching the codebase's other holder checks.
func samePath(a, b string) bool {
	return resolvePath(a) == resolvePath(b)
}

func resolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// isUnder reports whether path is at or below root (symlink-resolved).
func isUnder(path, root string) bool {
	if root == "" {
		return false
	}
	rp := resolvePath(path)
	rr := resolvePath(root)
	return rp == rr || strings.HasPrefix(rp, rr+string(filepath.Separator))
}
