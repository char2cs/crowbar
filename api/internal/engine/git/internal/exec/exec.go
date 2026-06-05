// Package exec runs git commands in a working directory and captures output.
package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(cmd, runErr),
	}
}

// GitWithEnv runs a git command in dir with extra environment variables appended
// to the current process environment.
func GitWithEnv(
	ctx context.Context,
	dir string,
	extraEnv []string,
	args ...string,
) Result {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(cmd, runErr),
	}
}

// GitWithStdin runs a git command with data piped to stdin.
func GitWithStdin(
	ctx context.Context,
	dir string,
	stdin string,
	args ...string,
) Result {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(cmd, runErr),
	}
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
