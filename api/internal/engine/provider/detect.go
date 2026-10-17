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

// DetectExecFn matches exec.CommandContext so callers can inject a test stub.
type DetectExecFn func(ctx context.Context, name string, args ...string) *exec.Cmd

// detectWaitDelay bounds how long Cmd.Wait blocks for I/O to drain after the
// context is cancelled. Without it a detection subprocess (git/gh/glab) whose
// pipes are held by a grandchild would keep its goroutine alive after a kill.
const detectWaitDelay = 10 * time.Second

// withWaitDelay wraps a DetectExecFn so every constructed Cmd carries a
// WaitDelay, guaranteeing a killed subprocess releases even with held pipes.
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

// DetectResult carries the detected provider kind and CLI availability.
type DetectResult struct {
	Kind    string // "github" | "gitlab" | "none"
	Enabled bool   // true if CLI present and authenticated
	Remote  string // the origin URL the kind was read from
}

// DefaultProtectedBranches is the fallback when the provider CLI is unavailable.
var DefaultProtectedBranches = []string{"main", "develop", "master"}

// FallbackProtectedBranches returns the config-list fallback when the provider
// CLI is absent or unauthenticated.
func FallbackProtectedBranches() []string {
	out := make([]string, len(DefaultProtectedBranches))
	copy(out, DefaultProtectedBranches)
	return out
}

// detectWithExec returns the provider kind and whether the CLI is available and
// authed. On any detection error it degrades to kind="none"/enabled=false
// rather than propagating the error.
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

// repoKey names the repository a worktree belongs to: its common git dir, which
// every linked worktree shares and where origin is configured. Any other path
// keys as itself.
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

// linkedCommonDir follows a linked worktree's ".git" file ("gitdir: <dir>") and
// that dir's "commondir" back to the main repository's git dir.
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

// detectRemoteOrigin returns the URL of the "origin" remote for repoPath.
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

// kindFromURL derives the provider kind from the remote origin URL.
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

// cliAuthed checks whether the provider CLI is installed and authenticated.
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

	// binpath.Resolve: the packaged .app daemon inherits launchd's minimal
	// PATH, which misses Homebrew's /opt/homebrew/bin where gh/glab usually
	// live — without this, detection marks the provider disabled and PR
	// statuses never sync in the packaged app.
	cmd := execFn(ctx, binpath.Resolve(cli), "auth", "status")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return err == nil
}
