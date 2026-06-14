// Package worktreepath derives deterministic filesystem paths for git
// worktrees and per-repo directories, all rooted under ~/.crowbar.
package worktreepath

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// For returns the worktree directory for workspaceID under crowbarHome.
//
// Path: <crowbarHome>/projects/<host>/<owner>/<repo>/workspaces/<workspaceID>
//
// remoteURL accepts HTTPS (https://github.com/owner/repo.git) and SSH
// (git@github.com:owner/repo.git) formats. An empty or unrecognised URL
// returns an error.
func For(crowbarHome, remoteURL, workspaceID string) (string, error) {
	dir, err := RepoDir(crowbarHome, remoteURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspaces", workspaceID), nil
}

// RepoDir returns the per-repo directory under crowbarHome/projects/.
//
// Example: https://github.com/acme/foo.git →
//
//	<crowbarHome>/projects/github.com/acme/foo
func RepoDir(crowbarHome, remoteURL string) (string, error) {
	rel, err := repoRelPath(remoteURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(crowbarHome, "projects", rel), nil
}

// DefaultCrowbarHome returns ~/.crowbar, the production root for all
// Crowbar-managed state.
func DefaultCrowbarHome() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("crowbar home: %w", err)
	}
	return filepath.Join(h, ".crowbar"), nil
}

// repoRelPath parses a git remote URL into <host>/<owner>/<repo>.
// It accepts HTTPS and SSH URL formats and strips any trailing ".git".
func repoRelPath(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("worktreepath: empty remote URL")
	}
	rawURL = strings.TrimSuffix(rawURL, ".git")

	// SSH: git@github.com:owner/repo
	if strings.HasPrefix(rawURL, "git@") {
		rest := rawURL[4:]
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return "", fmt.Errorf("worktreepath: invalid SSH URL: %q", rawURL)
		}
		host := rest[:idx]
		path := strings.TrimPrefix(rest[idx+1:], "/")
		return host + "/" + path, nil
	}

	// HTTPS: https://github.com/owner/repo
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("worktreepath: unrecognised remote URL: %q", rawURL)
	}
	path := strings.TrimPrefix(u.Path, "/")
	return u.Host + "/" + path, nil
}
