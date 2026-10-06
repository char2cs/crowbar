// Package handlers serves the daemon's log console endpoint.
package handlers

import "github.com/char2cs/crowbar/api/internal/core/logring"

// Handlers serves the console endpoints.
type Handlers struct {
	logs logring.Ring
}

// New returns Handlers that stream the records of logs.
func New(
	logs logring.Ring,
) *Handlers {
	return &Handlers{logs: logs}
}
