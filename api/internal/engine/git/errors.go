package git

import (
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// Sentinel errors returned by the git engine. Callers use errors.Is to
// distinguish failure modes and map them to structured API responses.
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

// ErrStaleHunk re-exports the diff subpackage's stale-hunk sentinel so callers
// outside the git engine (which cannot import the internal diff package) can
// match it with errors.Is. A hunk stage/unstage raises it when hunkID no longer
// matches any current diff hunk (04 §4); the API maps it to HTTP 409.
var ErrStaleHunk = diff.ErrStaleHunk

// ErrFileTooLarge re-exports the diff subpackage's size-ceiling sentinel so the
// usecase can tell "too big to expand" from a failure.
var ErrFileTooLarge = diff.ErrFileTooLarge
