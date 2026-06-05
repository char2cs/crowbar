// Package remote provides utilities for parsing Git remote URLs.
package remote

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ExecCommandFn matches exec.CommandContext so callers can pass their stub.
type ExecCommandFn func(ctx context.Context, name string, args ...string) *exec.Cmd

// Slug extracts the "owner/repo" slug from the git remote origin URL.
// The execFn parameter allows callers to inject a test stub.
func Slug(
	ctx context.Context,
	repoPath string,
	execFn ExecCommandFn,
) (string, error) {
	cmd := execFn(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = repoPath

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("remote: get-url: %w", err)
	}

	url := strings.TrimSpace(out.String())
	slug, err := slugFromURL(url)
	if err != nil {
		return "", fmt.Errorf("remote: slug: %w", err)
	}
	return slug, nil
}

// slugFromURL parses HTTPS and SSH remote URLs into "owner/repo".
func slugFromURL(
	url string,
) (string, error) {
	url = strings.TrimSuffix(url, ".git")

	// SSH: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
	}

	// HTTPS: https://github.com/owner/repo
	if idx := strings.Index(url, "://"); idx >= 0 {
		path := url[idx+3:]
		slash := strings.Index(path, "/")
		if slash >= 0 {
			return path[slash+1:], nil
		}
	}

	return "", fmt.Errorf("unrecognised remote URL format: %q", url)
}
