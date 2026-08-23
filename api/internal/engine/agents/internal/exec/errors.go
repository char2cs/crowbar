package exec

import "errors"

var (
	ErrOutputLimit = errors.New("agents: provider command exceeded its output limit")

	ErrCommandUnavailable = errors.New("agents: provider command is unavailable")

	ErrCommandFailed = errors.New("agents: provider command failed")
)
