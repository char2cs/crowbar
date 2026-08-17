package exec

import "errors"

var (
	// ErrOutputLimit means the command wrote more than its declared ceiling. The
	// command is killed at that point rather than buffered further.
	ErrOutputLimit = errors.New("agents: provider command exceeded its output limit")
	// ErrCommandUnavailable means the provider executable could not be found.
	ErrCommandUnavailable = errors.New("agents: provider command is unavailable")
	// ErrCommandFailed means the command ran and exited non-zero.
	ErrCommandFailed = errors.New("agents: provider command failed")
)
