package provider

import (
	"context"
	"os/exec"
	"time"
)

// NewWithExec builds the engine over a caller-supplied exec, so black-box tests
// can decide which subprocesses exist.
func NewWithExec(
	execFn func(ctx context.Context, name string, args ...string) *exec.Cmd,
	now func() time.Time,
) Engine {
	return newEngine(execFn, now)
}
