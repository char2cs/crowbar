// Package exec runs git commands in a working directory and captures output.
//
// Every invocation sets GIT_OPTIONAL_LOCKS=0 so read-only commands never take
// .git/index.lock, which would race user mutations on every watcher recompute.
// Mutations that hit a held lock are retried with bounded backoff, and a provably
// stale lock is removed (see lock_retry.go).
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/perf"
)

const optionalLocksOffEnv = "GIT_OPTIONAL_LOCKS=0"

// gitBin and recoverGit pick the binary a git invocation exec's (skipping the
// macOS shim); they are variables so tests can drive the vanished-binary path.
var (
	gitBin     = binpath.Git
	recoverGit = binpath.RecoverGit
)

// GitOpTimeout bounds every git subprocess whose caller did not bound it: one
// that never returns holds the per-repo mutex and stops every git operation on
// that clone. A minute is far above any healthy local git command.
const GitOpTimeout = 60 * time.Second

// waitDelay bounds Wait after cancellation: a git that ignores SIGTERM is then
// killed, and the pipes its children (ssh, credential helpers) still hold open
// are closed, so a cancelled command always returns.
const waitDelay = 3 * time.Second

func gitEnv() []string {
	return append(os.Environ(), optionalLocksOffEnv)
}

// GitError carries structured exit information for errors.Is / errors.As matching.
type GitError struct {
	Op       string
	ExitCode int
	Message  string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("%s: exit %d: %s", e.Op, e.ExitCode, e.Message)
}

// Detail is what git said, without its hints: the failure as a user reads it.
func (e *GitError) Detail() string {
	var lines []string
	for _, line := range strings.Split(e.Message, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "hint:") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// Result holds the captured output of a git invocation.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Git runs a git command in dir and returns its output.
// It never returns an error for non-zero exits; callers inspect ExitCode.
func Git(
	ctx context.Context,
	dir string,
	args ...string,
) Result {
	return runWithLockRetry(ctx, func() Result {
		return run(ctx, dir, "", false, args...)
	})
}

// GitWithStdin runs a git command with data piped to stdin.
func GitWithStdin(
	ctx context.Context,
	dir string,
	stdin string,
	args ...string,
) Result {
	return runWithLockRetry(ctx, func() Result {
		return run(ctx, dir, stdin, true, args...)
	})
}

// subcommandName returns the git subcommand to bucket a timing sample under, skipping
// leading global flags so one subcommand does not split across sample names.
func subcommandName(
	args []string,
) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-c" {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return "unknown"
}

// run is the single funnel for every git invocation, so one measurement point covers the
// whole subprocess surface; while the perf ring is disarmed it costs one atomic load.
func run(
	ctx context.Context,
	dir string,
	stdin string,
	hasStdin bool,
	args ...string,
) Result {
	if !perf.Enabled() {
		return runInner(ctx, dir, stdin, hasStdin, args...)
	}
	start := time.Now()
	r := runInner(ctx, dir, stdin, hasStdin, args...)
	perf.Record("git."+subcommandName(args), time.Since(start))
	return r
}

// runInner runs the invocation once and, only if the subprocess never started, lets the
// resolver notice a vanished binary and runs it again: nothing can have been applied.
func runInner(
	ctx context.Context,
	dir string,
	stdin string,
	hasStdin bool,
	args ...string,
) Result {
	bctx, cancel := boundedContext(ctx)
	defer cancel()

	r, started := execGit(bctx, gitBin(), dir, stdin, hasStdin, args)
	if !started && recoverGit() {
		r, _ = execGit(bctx, gitBin(), dir, stdin, hasStdin, args)
	}
	return classifyTimeout(bctx.Err(), ctx.Err(), r)
}

// execGit also reports whether the subprocess started, which gates runInner's
// retry. That is cmd.Process, not cmd.ProcessState: Wait leaves ProcessState nil
// when its own wait fails, which would re-run a mutation that already ran.
func execGit(
	ctx context.Context,
	bin string,
	dir string,
	stdin string,
	hasStdin bool,
	args []string,
) (Result, bool) {
	cmd := command(ctx, bin, dir, args)
	if hasStdin {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	r := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(cmd, runErr),
	}
	// A signalled or never-started git leaves no stderr of its own; surface the
	// run error ("signal: terminated", "context canceled") in its place.
	if r.ExitCode != 0 && r.Stderr == "" && runErr != nil {
		r.Stderr = runErr.Error()
	}
	return r, cmd.Process != nil
}

// command builds every git subprocess in the package, buffered or streamed, so
// they share one environment and one cancel policy.
func command(
	ctx context.Context,
	bin string,
	dir string,
	args []string,
) *exec.Cmd {
	//nolint:gosec // G204: running git with caller-supplied args is the purpose of this package.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	cmd.Cancel = func() error { return terminate(cmd.Process) }
	cmd.WaitDelay = waitDelay
	return cmd
}

// terminate ends a cancelled git with SIGTERM, on which git removes its own
// lockfiles; a SIGKILL would strand them for every later command. Where SIGTERM
// is unsupported (Windows) it kills.
func terminate(
	proc *os.Process,
) error {
	err := proc.Signal(syscall.SIGTERM)
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return proc.Kill()
}

// boundedContext applies GitOpTimeout to an unbounded invocation. A caller deadline is
// kept, since execNet runs network commands here under far longer bounds.
func boundedContext(
	ctx context.Context,
) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, GitOpTimeout)
}

// classifyTimeout rewrites a deadline-ended git's opaque "signal: terminated"
// as an explicit timeout, as execNet does. A deliberate cancellation keeps
// "context canceled", which is how a caller tells an abandoned git from a slow one.
func classifyTimeout(
	boundedErr error,
	parentErr error,
	r Result,
) Result {
	if r.ExitCode == 0 || !errors.Is(boundedErr, context.DeadlineExceeded) {
		return r
	}
	if parentErr != nil {
		r.Stderr = "git operation timed out: " + parentErr.Error()
		return r
	}
	r.Stderr = "git operation timed out after " + GitOpTimeout.String()
	return r
}

// RequireSuccess returns an error if the result has a non-zero exit code.
func RequireSuccess(
	op string,
	r Result,
) error {
	if r.ExitCode == 0 {
		return nil
	}
	msg := strings.TrimSpace(r.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(r.Stdout)
	}
	return &GitError{Op: op, ExitCode: r.ExitCode, Message: msg}
}

func exitCode(
	cmd *exec.Cmd,
	runErr error,
) int {
	if runErr == nil {
		return 0
	}
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	return 1
}
