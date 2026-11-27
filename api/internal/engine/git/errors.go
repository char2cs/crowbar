package git

import (
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

var (
	ErrConflict               = problem.New(problem.Conflict, "git: conflict")
	ErrRejectedNonFastForward = problem.New(problem.Conflict, "git: rejected_non_fast_forward")
	ErrNothingToCommit        = problem.New(problem.Conflict, "git: nothing_to_commit")
	ErrDirtyTree              = problem.New(problem.Conflict, "git: dirty_tree")
	ErrHasChildren            = problem.New(problem.Conflict, "git: has_children")
	ErrAuthFailed             = problem.New(problem.Forbidden, "git: auth_failed")
	ErrBranchAlreadyExists    = problem.New(problem.Conflict, "git: branch_already_exists")
	ErrBranchNotFound         = problem.New(problem.NotFound, "git: branch_not_found")
	ErrNoRemote               = problem.New(problem.InvalidArgument, "git: no_remote")
	ErrNonFastForward         = problem.New(problem.Conflict, "git: non_fast_forward")
)

var ErrStaleHunk = diff.ErrStaleHunk

var ErrFileTooLarge = diff.ErrFileTooLarge

type errorRule struct {
	exitCode int
	contains []string
	sentinel error
}

var errorRules = []errorRule{
	{1, []string{"conflict"}, ErrConflict},
	{1, []string{"nothing to commit"}, ErrNothingToCommit},
	{1, []string{"rejected", "non-fast-forward"}, ErrRejectedNonFastForward},
	{1, []string{"not possible to fast-forward"}, ErrNonFastForward},
	{1, []string{"your local changes"}, ErrDirtyTree},
	{1, []string{"please commit or stash"}, ErrDirtyTree},

	{128, []string{"authentication failed"}, ErrAuthFailed},
	{128, []string{"could not read username"}, ErrAuthFailed},
	{128, []string{"already exists"}, ErrBranchAlreadyExists},
	{128, []string{"already checked out"}, ErrBranchAlreadyExists},
	{128, []string{"already used by worktree"}, ErrBranchAlreadyExists},
	{128, []string{"did not match any"}, ErrBranchNotFound},
	{128, []string{"unknown revision or path"}, ErrBranchNotFound},
	{128, []string{"does not appear to be a git repository"}, ErrNoRemote},
	{128, []string{"no remote configured"}, ErrNoRemote},
	{128, []string{"refusing to merge unrelated histories"}, ErrNonFastForward},
	{128, []string{"your local changes"}, ErrDirtyTree},
	{128, []string{"please commit or stash"}, ErrDirtyTree},
}

func matchRules(exitCode int, out string) error {
	lower := strings.ToLower(out)
	for _, r := range errorRules {
		if r.exitCode != exitCode {
			continue
		}
		if allPresent(lower, r.contains) {
			return r.sentinel
		}
	}
	return nil
}

func allPresent(s string, patterns []string) bool {
	for _, p := range patterns {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func classifyGitError(op string, r gitexec.Result) error {
	base := gitexec.RequireSuccess(op, r)
	if base == nil {
		return nil
	}
	if sentinel := matchRules(r.ExitCode, r.Stderr+r.Stdout); sentinel != nil {
		return fmt.Errorf("%w: %w", sentinel, base)
	}
	return base
}

func reclassifyError(err error) error {
	if err == nil {
		return nil
	}
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		return err
	}
	if sentinel := matchRules(gitErr.ExitCode, gitErr.Message); sentinel != nil {
		return fmt.Errorf("%w: %w", sentinel, err)
	}
	return err
}
