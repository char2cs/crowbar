// Package exec runs git commands in a working directory and captures output.
//
// Every invocation runs with GIT_OPTIONAL_LOCKS=0 so read-only commands
// (status, diff, log, ...) never take .git/index.lock opportunistically; the
// fs watcher recomputes status on every shared-.git ref event, and an
// opportunistic index refresh there races user mutations (git restore et al.)
// for the same lock. Mutations that still hit a held index.lock — user
// terminals and shell prompts legitimately take it for short windows — are
// retried with a bounded backoff, and a lock left behind by a crashed git
// process is removed once it is provably stale (see lock_retry.go).
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/perf"
)

const optionalLocksOffEnv = "GIT_OPTIONAL_LOCKS=0"

// gitBin and recoverGit are the package's only view of which binary a git
// invocation exec's. They are variables so the tests can drive the
// vanished-binary recovery path without depending on what is installed on the
// machine running them; production always holds the binpath pair.
//
// Nothing else about an invocation changes: same argv, same environment, same
// working directory. Only the file that gets exec'd differs, and only to skip
// the macOS shim in front of it.
var (
	gitBin     = binpath.Git
	recoverGit = binpath.RecoverGit
)

// GitOpTimeout bounds every non-network git subprocess whose caller did not
// bound it already. Only the network commands were ever bounded (engine.go's
// execNet), on the reasoning that a dead remote stalls git for the OS
// retransmission timeout while the per-repo mutex is held — but the wedge that
// argument describes is not specific to the network. Any git subprocess that
// does not return holds the same mutex and stops every git operation on that
// clone, and local git has its own ways of not returning: a filesystem that
// stops answering, a repo large enough that a diff outlives the request that
// asked for it, a hook or credential helper waiting on input nobody will send.
//
// A minute is far above any healthy local git command and well below the point
// where a user has concluded the app is dead.
const GitOpTimeout = 60 * time.Second

// waitDelay bounds how long Wait blocks after a context-driven kill. Wait
// normally still waits until every process holding the stdout/stderr pipes
// exits — and git's own children (ssh, git-remote-https, credential helpers)
// can outlive the killed git and hold them open indefinitely. Forcibly closing
// the pipes shortly after cancellation is what makes a cancelled command
// actually return instead of trading one unbounded hang for another.
const waitDelay = 3 * time.Second

func gitEnv(
	extraEnv []string,
) []string {
	env := append(os.Environ(), optionalLocksOffEnv)
	return append(env, extraEnv...)
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
		return run(ctx, dir, nil, "", false, args...)
	})
}

// GitWithEnv runs a git command in dir with extra environment variables appended
// to the current process environment.
func GitWithEnv(
	ctx context.Context,
	dir string,
	extraEnv []string,
	args ...string,
) Result {
	return runWithLockRetry(ctx, func() Result {
		return run(ctx, dir, extraEnv, "", false, args...)
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
		return run(ctx, dir, nil, stdin, true, args...)
	})
}

// subcommandName returns the git subcommand to bucket a timing sample under,
// skipping the leading global flags (`-c key=value`, `--git-dir=…`) some call
// sites pass. Without the skip, one hot subcommand would split across as many
// sample names as there are flag prefixes in front of it.
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

// run wraps every git invocation in the package — Git, GitWithEnv and
// GitWithStdin all funnel through it — so a single measurement point covers the
// whole subprocess surface. While the perf ring is disarmed this costs one
// atomic load and delegates untouched.
func run(
	ctx context.Context,
	dir string,
	extraEnv []string,
	stdin string,
	hasStdin bool,
	args ...string,
) Result {
	if !perf.Enabled() {
		return runInner(ctx, dir, extraEnv, stdin, hasStdin, args...)
	}
	start := time.Now()
	r := runInner(ctx, dir, extraEnv, stdin, hasStdin, args...)
	perf.Record("git."+subcommandName(args), time.Since(start))
	return r
}

// runInner runs the invocation once and, only when the subprocess never
// started, gives the resolver a chance to notice its cached binary has gone and
// runs it again. The retry is safe precisely because the first attempt never
// started: no bytes were produced, nothing was mutated, and the command cannot
// have been half-applied.
func runInner(
	ctx context.Context,
	dir string,
	extraEnv []string,
	stdin string,
	hasStdin bool,
	args ...string,
) Result {
	bctx, cancel := boundedContext(ctx)
	defer cancel()

	r, started := execGit(bctx, gitBin(), dir, extraEnv, stdin, hasStdin, args)
	if !started && recoverGit() {
		r, _ = execGit(bctx, gitBin(), dir, extraEnv, stdin, hasStdin, args)
	}
	return classifyTimeout(bctx.Err(), ctx.Err(), r)
}

// execGit reports whether the subprocess started alongside its result. A
// command that never started has no ProcessState, and that is the only signal
// that separates "this binary could not be exec'd" from "git ran and said no".
func execGit(
	ctx context.Context,
	bin string,
	dir string,
	extraEnv []string,
	stdin string,
	hasStdin bool,
	args []string,
) (Result, bool) {
	//nolint:gosec // G204: running git with caller-supplied args is the purpose of this package.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(extraEnv)
	cmd.WaitDelay = waitDelay
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
	// A subprocess killed by a signal (ctx cancel, OOM) or a fork/exec failure
	// exits with no git-produced stderr, leaving an opaque "exit -1: " error.
	// Surface the run error so the actual cause (e.g. "signal: killed",
	// "context canceled") reaches logs and the error envelope.
	if r.ExitCode != 0 && r.Stderr == "" && runErr != nil {
		r.Stderr = runErr.Error()
	}
	return r, cmd.ProcessState != nil
}

// boundedContext applies GitOpTimeout to an invocation the caller left
// unbounded. A caller that already set a deadline keeps it: execNet runs the
// network commands through this same path under its own, far longer bounds
// (three minutes for a transfer), and silently shortening those to the
// non-network timeout would abort legitimate fetches of large repos.
func boundedContext(
	ctx context.Context,
) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, GitOpTimeout)
}

// classifyTimeout replaces the opaque result of a deadline-killed subprocess
// with an explicit timeout, the way execNet already does for network commands:
// CommandContext kills the process, so what reaches the caller is "exit -1:
// signal: killed", which says nothing about the deadline that caused it.
//
// Only a deadline is rewritten. A deliberate cancellation keeps "context
// canceled", which is already the truth and is how the caller distinguishes a
// git operation it abandoned from one that ran too long.
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
