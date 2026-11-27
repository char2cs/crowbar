package shellenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
)

type ExecFn func(ctx context.Context, name string, args ...string) *exec.Cmd

const resolveTimeout = 5 * time.Second

func ApplyLoginShellPath(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	loginPath, err := LoginShellPath(ctx)
	if err != nil {
		return os.Getenv("PATH")
	}
	merged := mergePaths(loginPath, os.Getenv("PATH"))
	if err := os.Setenv("PATH", merged); err != nil {
		slog.WarnContext(ctx, "shellenv: apply the login-shell PATH", "err", err)
		return os.Getenv("PATH")
	}
	return merged
}

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
