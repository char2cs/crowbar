package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// Rebases, merges and pulls can stop midway. Only a stop on unmerged paths is
// the user's to resolve; any other stop is aborted so the worktree is left as
// it was found and the next command is not refused.

func (e *engine) Merge(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	before := detectInProgressOp(ctx, repoPath)
	// `--` end-of-options separator so a branch value can never be read as a git
	// option (argument injection); the usecase boundary also rejects leading-dash
	// operands as defense in depth.
	r := e.exec(ctx, repoPath, "merge", "--", branch)
	return e.settleStoppable(ctx, repoPath, before, "merge", r)
}

func (e *engine) Rebase(
	ctx context.Context,
	repoPath string,
	onto string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	before := detectInProgressOp(ctx, repoPath)
	// `--` end-of-options separator so an onto value can never be read as a git
	// option such as `--exec=<cmd>` (argument injection → arbitrary command
	// execution); the usecase boundary also rejects leading-dash operands.
	r := e.exec(ctx, repoPath, "rebase", "--", onto)
	return e.settleStoppable(ctx, repoPath, before, "rebase", r)
}

func (e *engine) Pull(
	ctx context.Context,
	repoPath string,
	mode string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	flag := "--no-rebase"
	if mode == "rebase" {
		flag = "--rebase"
	}
	before := detectInProgressOp(ctx, repoPath)
	r, timeoutErr := e.runNet(ctx, "pull", netTransferTimeout, repoPath, "pull", flag)
	if timeoutErr != nil {
		return e.leaveAsFound(ctx, repoPath, before, timeoutErr, nil)
	}
	return e.settleStoppable(ctx, repoPath, before, "pull", r)
}

// settleStoppable classifies r like classifyGitError, except that a conflict is
// read from the index (unmerged paths) and any other stop is aborted.
func (e *engine) settleStoppable(
	ctx context.Context,
	repoPath string,
	before string,
	op string,
	r gitexec.Result,
) error {
	base := gitexec.RequireSuccess(op, r)
	if base == nil {
		return nil
	}
	return e.leaveAsFound(ctx, repoPath, before, base, matchRules(r.ExitCode, r.Stderr+r.Stdout))
}

// leaveAsFound settles a failed command that started with before in progress.
// A pre-existing operation is the user's and is never touched. Probes and the
// abort outlive a cancelled request, which can kill git midway too.
func (e *engine) leaveAsFound(
	ctx context.Context,
	repoPath string,
	before string,
	base error,
	sentinel error,
) error {
	if before != "" {
		return withSentinel(sentinel, base)
	}
	ctx = context.WithoutCancel(ctx)
	unmerged, err := e.hasUnmergedPaths(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("%w; reading the index: %w", withSentinel(sentinel, base), err)
	}
	if unmerged {
		return fmt.Errorf("%w: %w", ErrConflict, base)
	}
	if errors.Is(sentinel, ErrConflict) {
		sentinel = nil
	}
	failed := withSentinel(sentinel, base)
	if detectInProgressOp(ctx, repoPath) == "" {
		return failed
	}
	if abortErr := e.operationAbort(ctx, repoPath); abortErr != nil {
		return fmt.Errorf("%w; restoring the worktree: %w", failed, abortErr)
	}
	return failed
}

func (e *engine) hasUnmergedPaths(ctx context.Context, repoPath string) (bool, error) {
	r := e.exec(ctx, repoPath, "ls-files", "--unmerged")
	if err := gitexec.RequireSuccess("ls-files --unmerged", r); err != nil {
		return false, err
	}
	return strings.TrimSpace(r.Stdout) != "", nil
}

func withSentinel(sentinel, base error) error {
	if sentinel == nil {
		return base
	}
	return fmt.Errorf("%w: %w", sentinel, base)
}
