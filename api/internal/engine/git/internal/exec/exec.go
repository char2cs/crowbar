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

var (
	gitBin     = binpath.Git
	recoverGit = binpath.RecoverGit
)

const GitOpTimeout = 60 * time.Second

const waitDelay = 3 * time.Second

func gitEnv() []string {
	return append(os.Environ(), optionalLocksOffEnv)
}

type GitError struct {
	Op       string
	ExitCode int
	Message  string
}

func (e *GitError) Error() string {
	return fmt.Sprintf("%s: exit %d: %s", e.Op, e.ExitCode, e.Message)
}

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

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func Git(
	ctx context.Context,
	dir string,
	args ...string,
) Result {
	return runWithLockRetry(ctx, func() Result {
		return run(ctx, dir, "", false, args...)
	})
}

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
	if r.ExitCode != 0 && r.Stderr == "" && runErr != nil {
		r.Stderr = runErr.Error()
	}
	return r, cmd.Process != nil
}

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

func terminate(
	proc *os.Process,
) error {
	err := proc.Signal(syscall.SIGTERM)
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return proc.Kill()
}

func boundedContext(
	ctx context.Context,
) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, GitOpTimeout)
}

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
