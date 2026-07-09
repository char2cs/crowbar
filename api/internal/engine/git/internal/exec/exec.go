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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const optionalLocksOffEnv = "GIT_OPTIONAL_LOCKS=0"

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

func run(
	ctx context.Context,
	dir string,
	extraEnv []string,
	stdin string,
	hasStdin bool,
	args ...string,
) Result {
	//nolint:gosec // G204: running git with caller-supplied args is the purpose of this package.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), optionalLocksOffEnv)
	cmd.Env = append(cmd.Env, extraEnv...)
	// After a context-driven kill, Wait normally still blocks until every
	// process holding the stdout/stderr pipes exits — and git's own children
	// (ssh, git-remote-https, credential helpers) can outlive the killed git
	// and hold them open indefinitely. WaitDelay forcibly closes the pipes
	// shortly after cancellation so a timed-out network command actually
	// returns instead of trading one unbounded hang for another.
	cmd.WaitDelay = 3 * time.Second
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
