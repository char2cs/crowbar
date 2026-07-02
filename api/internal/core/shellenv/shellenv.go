// Package shellenv resolves the user's login-shell PATH and applies it to the
// daemon process. A daemon launched by macOS launchd (the packaged .app)
// inherits a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin) that
// misses Homebrew, npm, go and cargo install locations — so every bare-name
// exec (gh, glab, language servers) and every spawned PTY child sees a
// crippled environment. Resolving the login shell's PATH once at startup and
// applying it process-wide fixes all of them in one place (the same approach
// VS Code uses on macOS).
package shellenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
)

// ExecFn matches exec.CommandContext so tests can inject a stub.
type ExecFn func(ctx context.Context, name string, args ...string) *exec.Cmd

// resolveTimeout bounds the login-shell spawn: a hanging ~/.zprofile (network
// calls, prompts) must never block daemon startup.
const resolveTimeout = 5 * time.Second

// ApplyLoginShellPath resolves the login-shell PATH and applies it to the
// process via os.Setenv, merged with (and taking precedence over) the current
// PATH so no inherited entry is ever lost. It degrades gracefully: on any
// resolution failure the current PATH is left untouched. Returns the PATH the
// process ends up with.
//
// Setting the PROCESS PATH (not cmd.Env) matters: Go's exec.Command resolves
// bare binary names against the process PATH at construction time — a PATH
// placed only in cmd.Env is ignored for lookup.
func ApplyLoginShellPath(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	loginPath, err := LoginShellPath(ctx)
	if err != nil {
		return os.Getenv("PATH")
	}
	merged := mergePaths(loginPath, os.Getenv("PATH"))
	_ = os.Setenv("PATH", merged)
	return merged
}

// LoginShellPath returns the PATH exported by the user's login shell.
func LoginShellPath(ctx context.Context) (string, error) {
	return loginShellPath(ctx, exec.CommandContext, runtime.GOOS, os.Getenv)
}

func loginShellPath(
	ctx context.Context,
	execFn ExecFn,
	goos string,
	getenv func(string) string,
) (string, error) {
	shell := resolveShell(ctx, execFn, goos, getenv)

	var stdout, stderr bytes.Buffer
	// stderr is discarded so profile print statements don't pollute stdout.
	cmd := execFn(ctx, shell, "-l", "-c", `printf '%s' "$PATH"`)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("shellenv: %s -l: %w", shell, err)
	}

	path := strings.TrimSpace(stdout.String())
	if path == "" {
		return "", errors.New("shellenv: login shell exported an empty PATH")
	}
	return path, nil
}

// resolveShell picks the user's shell: $SHELL when set, else (on macOS) the
// account's shell from Directory Services — launchd's GUI-session environment
// does not guarantee SHELL is set — else /bin/sh.
func resolveShell(
	ctx context.Context,
	execFn ExecFn,
	goos string,
	getenv func(string) string,
) string {
	if s := getenv("SHELL"); s != "" {
		return s
	}
	if goos == "darwin" {
		if s := dsclUserShell(ctx, execFn); s != "" {
			return s
		}
	}
	return "/bin/sh"
}

// dsclUserShell reads the current user's shell from Directory Services.
// Output format: "UserShell: /bin/zsh". Returns "" on any failure.
func dsclUserShell(
	ctx context.Context,
	execFn ExecFn,
) string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	var out bytes.Buffer
	cmd := execFn(ctx, "dscl", ".", "-read", "/Users/"+u.Username, "UserShell")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	_, shell, ok := strings.Cut(out.String(), "UserShell:")
	if !ok {
		return ""
	}
	return strings.TrimSpace(shell)
}

// mergePaths joins primary and secondary PATH strings, keeping order and
// dropping duplicates — primary entries win the ordering.
func mergePaths(
	primary string,
	secondary string,
) string {
	seen := map[string]bool{}
	var out []string
	for _, entry := range append(
		strings.Split(primary, string(os.PathListSeparator)),
		strings.Split(secondary, string(os.PathListSeparator))...,
	) {
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		out = append(out, entry)
	}
	return strings.Join(out, string(os.PathListSeparator))
}
