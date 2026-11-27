package provider

import (
	"context"
	"os/exec"
	"time"
)

func NewWithExec(
	execFn func(ctx context.Context, name string, args ...string) *exec.Cmd,
	now func() time.Time,
) Engine {
	return newEngine(execFn, now)
}
