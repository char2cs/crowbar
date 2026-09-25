// Package provision checks an EXISTING branch — a protected branch at import,
// or a placeholder's branch on Retry/Detach — out into a fresh Crowbar-managed
// worktree. It is the one copy of that rule; the project import and the
// worktree hierarchy both call it (spec §7-D: the two copies had drifted).
package provision

import (
	"context"
	"fmt"
	"log/slog"
)

// Git is the git surface a provision needs.
type Git interface {
	RemoteTrackingBranchExists(ctx context.Context, repoPath, branch string) (bool, error)
	FetchRef(ctx context.Context, repoPath, branch string) error
	WorktreeAdd(ctx context.Context, repoPath, worktreePath, branch string) error
	WorktreeAddAtRef(ctx context.Context, repoPath, worktreePath, branch, startRef string) (string, error)
	SetUpstream(ctx context.Context, repoPath, branch string) error
	RevParse(ctx context.Context, repoPath, rev string) (string, error)
	MergeBase(ctx context.Context, repoPath, a, b string) (string, error)
}

// ExistingBranch checks branch out into a new worktree at path and returns the
// commit it landed on (the workspace's fork point).
//
// A branch origin has is checked out AT origin's ref (`git worktree add -B`),
// so the worktree holds the REMOTE branch even when the local ref has diverged
// (a leftover from before the folder was adopted, or a force-push): checking
// out the local ref imported the user's stale commits under a row claiming to
// be origin's branch. The local remote-tracking ref is read FIRST, so a repo
// with no remote never pays a network fetch under the per-clone lock; the fetch
// itself is best-effort (offline still checks out the local origin/<b>). The
// upstream is set explicitly — a -B checkout from a SHA sets none, and `git
// pull` in the worktree then has nothing to merge with.
//
// A branch origin does not have checks out from the local ref, the only
// content there is.
func ExistingBranch(
	ctx context.Context,
	git Git,
	repoPath string,
	branch string,
	path string,
) (string, error) {
	if onOrigin, err := git.RemoteTrackingBranchExists(ctx, repoPath, branch); err == nil && onOrigin {
		return fromOrigin(ctx, git, repoPath, branch, path)
	}
	if err := git.WorktreeAdd(ctx, repoPath, path, branch); err != nil {
		return "", fmt.Errorf("worktree add %q: %w", branch, err)
	}
	// The BRANCH head, explicitly: `git rev-parse <name>` prefers a tag of the
	// same name, which would record a wrong fork point.
	sha, err := git.RevParse(ctx, repoPath, "refs/heads/"+branch)
	if err != nil {
		// The worktree is valid; only the recorded fork point is missing, which
		// merge and diff math then cannot use. Said out loud, not swallowed.
		slog.WarnContext(ctx, "provision: could not resolve the branch tip; the fork point stays empty",
			"branch", branch, "err", err)
		return "", nil
	}
	return sha, nil
}

func fromOrigin(
	ctx context.Context,
	git Git,
	repoPath string,
	branch string,
	path string,
) (string, error) {
	if err := git.FetchRef(ctx, repoPath, branch); err != nil {
		slog.WarnContext(ctx, "provision: could not refresh origin branch; using the local remote-tracking ref",
			"branch", branch, "err", err)
	}
	// Read BEFORE the -B reset moves it, to report a local tip it leaves behind.
	localTip, _ := git.RevParse(ctx, repoPath, "refs/heads/"+branch)
	sha, err := git.WorktreeAddAtRef(ctx, repoPath, path, branch, "origin/"+branch)
	if err != nil {
		return "", fmt.Errorf("worktree add %q at origin: %w", branch, err)
	}
	WarnOnDiscardedLocalTip(ctx, git, repoPath, branch, localTip, sha)
	if err := git.SetUpstream(ctx, repoPath, branch); err != nil {
		slog.WarnContext(ctx, "provision: could not set upstream; pull/ahead-behind may not work",
			"branch", branch, "err", err)
	}
	return sha, nil
}

// WarnOnDiscardedLocalTip logs the local branch tip a -B reset just moved off,
// when that tip was NOT already contained in origin's. The reset moves a ref,
// it does not rewrite history — the commits stay reachable through the reflog —
// but a user with unpushed work on a same-named local branch deserves the SHA
// to recover it from. Purely diagnostic: every failure is ignored.
func WarnOnDiscardedLocalTip(
	ctx context.Context,
	git Git,
	repoPath string,
	branch string,
	localTip string,
	originTip string,
) {
	if localTip == "" || localTip == originTip {
		return
	}
	if base, err := git.MergeBase(ctx, repoPath, localTip, originTip); err == nil && base == localTip {
		return // a plain fast-forward; nothing was left behind
	}
	slog.WarnContext(ctx, "provision: local branch had diverged from origin and was reset to origin's tip (old tip recoverable via reflog)",
		"branch", branch, "old_local_tip", localTip, "origin_tip", originTip)
}
