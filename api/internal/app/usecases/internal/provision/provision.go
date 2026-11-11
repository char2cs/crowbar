// Package provision checks an existing branch (protected at import, or a placeholder's on Retry/Detach)
// out into a fresh managed worktree: the one copy of that rule, shared by import and hierarchy.
package provision

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
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

// ExistingBranch checks branch out at path and returns its commit (the fork point). A branch origin has
// is checked out at origin's ref with -B, so a diverged local ref is not imported as origin's, with the
// upstream set explicitly; the fetch is best-effort. A branch origin lacks checks out from the local ref.
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
	localTip, err := LocalTip(ctx, git, repoPath, branch)
	if err != nil {
		return "", err
	}
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

// LocalTip is branch's local head, or "" when there is no local branch. Any
// other failure is returned: a tip that cannot be read is not an absent one.
func LocalTip(
	ctx context.Context,
	git Git,
	repoPath string,
	branch string,
) (string, error) {
	tip, err := git.RevParse(ctx, repoPath, "refs/heads/"+branch)
	if errors.Is(err, gitengine.ErrBranchNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read local tip of %q: %w", branch, err)
	}
	return tip, nil
}

// WarnOnDiscardedLocalTip logs the local tip a -B reset moved off when origin did not contain it, so a
// user with unpushed work has the SHA to recover from the reflog. Diagnostic only; failures are ignored.
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
