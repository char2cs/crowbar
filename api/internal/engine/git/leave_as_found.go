package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// Rebases, merges and pulls can stop midway. Paths this command left unmerged
// are a conflict for the user; any other operation it started is aborted, and
// what was already in progress, staged or dirty is never touched. Operands
// follow `--` so a value is never read as an option such as --exec=<cmd>.

func (e *engine) Merge(ctx context.Context, repoPath, branch string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.stoppable(ctx, repoPath, "merge", "merge", "--", branch)
}

func (e *engine) Rebase(ctx context.Context, repoPath, onto string) error {
	defer e.lockRepo(ctx, repoPath)()
	return e.stoppable(ctx, repoPath, "rebase", "rebase", "--", onto)
}

func (e *engine) Pull(ctx context.Context, repoPath, mode string) error {
	defer e.lockRepo(ctx, repoPath)()
	flag := "--no-rebase"
	if mode == "rebase" {
		flag = "--rebase"
	}
	before := e.snapshot(ctx, repoPath)
	r, timeoutErr := e.runNet(ctx, "pull", netTransferTimeout, repoPath, "pull", flag)
	if timeoutErr != nil {
		return e.leaveAsFound(ctx, repoPath, before, timeoutErr, nil)
	}
	return e.settle(ctx, repoPath, before, "pull", r)
}

func (e *engine) stoppable(ctx context.Context, repoPath, op string, args ...string) error {
	before := e.snapshot(ctx, repoPath)
	return e.settle(ctx, repoPath, before, op, e.exec(ctx, repoPath, args...))
}

func (e *engine) settle(ctx context.Context, repoPath string, before worktreeState, op string, r gitexec.Result) error {
	base := gitexec.RequireSuccess(op, r)
	if base == nil {
		return nil
	}
	return e.leaveAsFound(ctx, repoPath, before, base, matchRules(r.ExitCode, r.Stderr+r.Stdout))
}

// stopMarkers are what a stopped operation leaves. AUTO_MERGE is not one: a
// merge refused over the user's own changes writes it too.
var stopMarkers = []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "SQUASH_HEAD", "CHERRY_PICK_HEAD"}

type worktreeState struct {
	markers  map[string]bool
	unmerged bool
	err      error
}

func (e *engine) snapshot(ctx context.Context, repoPath string) worktreeState {
	gitDir := resolveGitDir(ctx, repoPath)
	s := worktreeState{markers: map[string]bool{}}
	for _, m := range stopMarkers {
		s.markers[m] = fileExists(filepath.Join(gitDir, m))
	}
	r := e.exec(ctx, repoPath, "ls-files", "--unmerged")
	s.err = gitexec.RequireSuccess("ls-files --unmerged", r)
	s.unmerged = strings.TrimSpace(r.Stdout) != ""
	return s
}

func (s worktreeState) startedSince(before worktreeState) bool {
	for m, present := range s.markers {
		if present && !before.markers[m] {
			return true
		}
	}
	return false
}

// leaveAsFound settles a failed stoppable command. The probes and the abort
// outlive a cancelled request, which can kill git midway too.
func (e *engine) leaveAsFound(ctx context.Context, repoPath string, before worktreeState, base, sentinel error) error {
	ctx = context.WithoutCancel(ctx)
	after := e.snapshot(ctx, repoPath)
	failed := base
	if sentinel != nil && !errors.Is(sentinel, ErrConflict) {
		failed = fmt.Errorf("%w: %w", sentinel, base)
	}
	if err := errors.Join(before.err, after.err); err != nil {
		return fmt.Errorf("%w; reading the index: %w", failed, err)
	}
	if after.unmerged && !before.unmerged {
		return fmt.Errorf("%w: %w", ErrConflict, base)
	}
	if !after.startedSince(before) {
		return failed
	}
	if abortErr := e.operationAbort(ctx, repoPath); abortErr != nil {
		return fmt.Errorf("%w; restoring the worktree: %w", failed, abortErr)
	}
	return failed
}
