package provider

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DetectExecFn matches exec.CommandContext so callers can inject a test stub.
type DetectExecFn func(ctx context.Context, name string, args ...string) *exec.Cmd

// DetectResult carries the detected provider kind and CLI availability.
type DetectResult struct {
	Kind    string // "github" | "gitlab" | "none"
	Enabled bool   // true if CLI present and authenticated
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

// Detect returns the provider kind and whether the CLI is available+authed.
// On any detection error the function degrades gracefully: it returns
// kind="none"/enabled=false rather than propagating the error.
func Detect(
	ctx context.Context,
	repoPath string,
) (DetectResult, error) {
	return DetectWithExec(ctx, repoPath, exec.CommandContext)
}

// DetectWithExec is like Detect but accepts an injectable exec function.
// Intended for tests.
func DetectWithExec(
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
	return DetectResult{Kind: kind, Enabled: enabled}, nil
}

// detectRemoteOrigin returns the URL of the "origin" remote for repoPath.
func detectRemoteOrigin(
	ctx context.Context,
	repoPath string,
	execFn DetectExecFn,
) (string, error) {
	cmd := execFn(ctx, "git", "remote", "get-url", "origin")
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

	cmd := execFn(ctx, cli, "auth", "status")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return err == nil
}
