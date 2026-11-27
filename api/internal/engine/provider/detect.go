package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

type DetectExecFn func(ctx context.Context, name string, args ...string) *exec.Cmd

const detectWaitDelay = 10 * time.Second

func withWaitDelay(
	execFn DetectExecFn,
) DetectExecFn {
	return func(
		ctx context.Context,
		name string,
		args ...string,
	) *exec.Cmd {
		cmd := execFn(ctx, name, args...)
		cmd.WaitDelay = detectWaitDelay
		return cmd
	}
}

type DetectResult struct {
	Kind    string
	Enabled bool
	Remote  string
}

var DefaultProtectedBranches = []string{"main", "develop", "master"}

func FallbackProtectedBranches() []string {
	out := make([]string, len(DefaultProtectedBranches))
	copy(out, DefaultProtectedBranches)
	return out
}

func detectWithExec(
	ctx context.Context,
	repoPath string,
	execFn DetectExecFn,
) (DetectResult, error) {
	origin, err := detectRemoteOrigin(ctx, repoPath, execFn)
	if err != nil {
		return DetectResult{Kind: "none"}, nil
	}

	kind := kindFromURL(origin)
	if kind == "none" {
		return DetectResult{Kind: "none"}, nil
	}

	enabled := cliAuthed(ctx, kind, execFn)
	return DetectResult{Kind: kind, Enabled: enabled, Remote: origin}, nil
}

func repoKey(
	path string,
) string {
	gitDir := filepath.Join(path, ".git")
	if fi, err := os.Lstat(gitDir); err == nil && !fi.IsDir() {
		gitDir = linkedCommonDir(path, gitDir)
	}
	if real, err := filepath.EvalSymlinks(gitDir); err == nil {
		return real
	}
	return filepath.Clean(path)
}

func linkedCommonDir(
	path string,
	dotGit string,
) string {
	raw, err := os.ReadFile(dotGit) //nolint:gosec // G304: reads the .git pointer of a worktree the daemon manages
	if err != nil {
		return dotGit
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(string(raw), "gitdir:"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(path, gitDir)
	}
	common, err := os.ReadFile(filepath.Join(gitDir, "commondir")) //nolint:gosec // G304: inside the worktree's own git dir
	if err != nil {
		return gitDir
	}
	dir := strings.TrimSpace(string(common))
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(gitDir, dir)
}

func detectRemoteOrigin(
	ctx context.Context,
	repoPath string,
	execFn DetectExecFn,
) (string, error) {
	cmd := execFn(ctx, binpath.Git(), "remote", "get-url", "origin")
	cmd.Dir = repoPath

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("detect: remote origin: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

func kindFromURL(
	url string,
) string {
	lower := strings.ToLower(url)
	if strings.Contains(lower, "github.com") {
		return "github"
	}
	if strings.Contains(lower, "gitlab.com") || strings.Contains(lower, "gitlab") {
		return "gitlab"
	}
	return "none"
}

func cliAuthed(
	ctx context.Context,
	kind string,
	execFn DetectExecFn,
) bool {
	var cli string
	switch kind {
	case "github":
		cli = "gh"
	case "gitlab":
		cli = "glab"
	default:
		return false
	}

	cmd := execFn(ctx, binpath.Resolve(cli), "auth", "status")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return err == nil
}
