package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"sync"
	"time"
)

const waitDelay = 500 * time.Millisecond

type Acquire func(context.Context) (release func(), err error)

type Options struct {
	Executable string
	Cwd        string
	Env        []string
	MaxStdout  int
	MaxStderr  int
	Acquire    Acquire
}

type Runner struct {
	opts   Options
	stdout *budget
	stderr *budget
}

func New(opts Options) *Runner {
	return &Runner{
		opts:   opts,
		stdout: newBudget(opts.MaxStdout),
		stderr: newBudget(opts.MaxStderr),
	}
}

func (r *Runner) Run(ctx context.Context, argv []string) ([]byte, error) {
	if r.opts.Acquire != nil {
		release, err := r.opts.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
	}

	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := osexec.CommandContext(commandCtx, r.opts.Executable, argv...) //nolint:gosec // see above
	cmd.Dir = r.opts.Cwd
	cmd.Env = append([]string(nil), r.opts.Env...)

	isolateProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessTree(cmd.Process) }
	cmd.WaitDelay = waitDelay

	stdout := newBoundedBuffer(r.stdout, cancel)
	stderr := newBoundedBuffer(r.stderr, cancel)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()

	if stdout.Exceeded() || stderr.Exceeded() {
		return nil, ErrOutputLimit
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if runErr != nil {
		return nil, classify(runErr)
	}
	return stdout.Bytes(), nil
}

func classify(err error) error {
	if errors.Is(err, osexec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return ErrCommandUnavailable
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{Code: exitErr.ExitCode()}
	}
	return ErrCommandFailed
}

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s (exit %d)", ErrCommandFailed, e.Code)
}

func (e *ExitError) Unwrap() error { return ErrCommandFailed }

func ExitCode(err error) (int, bool) {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}

type budget struct {
	mu        sync.Mutex
	remaining int
}

func newBudget(max int) *budget {
	return &budget{remaining: max}
}

func (b *budget) consume(requested int) (accepted int, exceeded bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if requested <= b.remaining {
		b.remaining -= requested
		return requested, false
	}
	accepted = b.remaining
	b.remaining = 0
	return accepted, true
}

type boundedBuffer struct {
	buf      bytes.Buffer
	budget   *budget
	exceeded bool
	cancel   context.CancelFunc
}

func newBoundedBuffer(b *budget, cancel context.CancelFunc) *boundedBuffer {
	return &boundedBuffer{budget: b, cancel: cancel}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.exceeded {
		return 0, ErrOutputLimit
	}
	accepted, exceeded := b.budget.consume(len(p))
	if accepted > 0 {
		_, _ = b.buf.Write(p[:accepted])
	}
	if exceeded {
		b.exceeded = true
		b.cancel()
		return accepted, ErrOutputLimit
	}
	return accepted, nil
}

func (b *boundedBuffer) Bytes() []byte {
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *boundedBuffer) Exceeded() bool { return b.exceeded }
