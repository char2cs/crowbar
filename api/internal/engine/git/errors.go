package git

import (
	"errors"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// Sentinel errors returned by the git engine. Callers use errors.Is to
// distinguish failure modes and map them to structured API responses.
var (
	ErrConflict               = errors.New("git: conflict")
	ErrRejectedNonFastForward = errors.New("git: rejected_non_fast_forward")
	ErrNothingToCommit        = errors.New("git: nothing_to_commit")
	ErrDirtyTree              = errors.New("git: dirty_tree")
	ErrHasChildren            = errors.New("git: has_children")
	ErrAuthFailed             = errors.New("git: auth_failed")
	ErrBranchAlreadyExists    = errors.New("git: branch_already_exists")
	ErrBranchNotFound         = errors.New("git: branch_not_found")
	ErrNoRemote               = errors.New("git: no_remote")
	ErrNonFastForward         = errors.New("git: non_fast_forward")
)

// ErrStaleHunk re-exports the diff subpackage's stale-hunk sentinel so callers
// outside the git engine (which cannot import the internal diff package) can
// match it with errors.Is. A hunk stage/unstage raises it when hunkID no longer
// matches any current diff hunk (04 §4); the API maps it to HTTP 409.
var ErrStaleHunk = diff.ErrStaleHunk
