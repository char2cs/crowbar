package git

import "errors"

// Sentinel errors returned by the git engine. Callers use errors.Is to
// distinguish failure modes and map them to structured API responses.
var (
	ErrConflict               = errors.New("git: conflict")
	ErrRejectedNonFastForward = errors.New("git: rejected_non_fast_forward")
	ErrNothingToCommit        = errors.New("git: nothing_to_commit")
	ErrDirtyTree              = errors.New("git: dirty_tree")
	ErrHasChildren            = errors.New("git: has_children")
	ErrAuthFailed             = errors.New("git: auth_failed")
)
