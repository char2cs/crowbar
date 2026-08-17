// Package exec runs one bounded provider command.
//
// Every provider probe in the engine — slash catalogues, model catalogues, effort
// enumeration, telemetry polling — is the same operation: run a fixed subcommand
// of a provider CLI, in the chat's worktree, under a timeout, with a ceiling on
// how much it may write, killing the whole process tree if it misbehaves. That
// operation lives here exactly once.
//
// No shell is ever started, argv is never a string, and stdout and stderr are
// discarded after mapping. Errors deliberately omit the executable, argv, cwd and
// output: a provider may print credentials or config locations on failure.
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

// waitDelay bounds how long a descendant that inherited the command's pipes may
// keep them open after the command itself exits. Without it such a descendant
// strands the read indefinitely.
const waitDelay = 500 * time.Millisecond

// Acquire is a caller-owned concurrency permit, taken immediately before every
// provider command and released after it exits.
//
// It exists so the engine stays independent of daemon policy while every
// concurrent probe shares one budget: a single inventory can fan out several
// detail commands, so without a shared gate N windows probing at once fork N
// times that fanout.
type Acquire func(context.Context) (release func(), err error)

// Options configures a Runner. Cwd must be an absolute, existing directory; the
// caller validates that, because what makes a directory acceptable is the
// caller's concern, not this package's.
type Options struct {
	Executable string
	Cwd        string
	Env        []string
	MaxStdout  int
	MaxStderr  int
	Acquire    Acquire
}

// Runner executes bounded provider commands. Its output budgets are shared across
// every command it runs, so a pipeline that fans out cannot exceed its ceiling by
// splitting the work.
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

// Run executes argv and returns stdout.
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

	// Launching a variable command IS this package's job: it runs a provider CLI
	// the descriptor named. What bounds it is that no shell is involved, argv is a
	// slice rather than a string, and the descriptor rules reject a templated or
	// forbidden command before it can reach here.
	cmd := osexec.CommandContext(commandCtx, r.opts.Executable, argv...) //nolint:gosec // see above
	cmd.Dir = r.opts.Cwd
	cmd.Env = append([]string(nil), r.opts.Env...)
	// Cancellation must reach the provider's OWN helpers, not just the process
	// Crowbar forked: a CLI that shells out leaves those children running under
	// the default Cancel, which signals one pid.
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

// ExitError reports a non-zero exit WITHOUT the command's output. Some probes
// read the code deliberately — an effort enumeration is driven by a sentinel
// argument that is expected to fail — so the code is worth carrying where the
// output is not.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s (exit %d)", ErrCommandFailed, e.Code)
}

func (e *ExitError) Unwrap() error { return ErrCommandFailed }

// ExitCode returns the exit status carried by err, if it carries one.
func ExitCode(err error) (int, bool) {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}

// budget is a byte allowance shared by every command a Runner runs.
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

// boundedBuffer accumulates output until the shared budget is spent, then kills
// the command rather than continuing to read it.
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
