package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

func fetchGithubAvatarBytes(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error) {
	url := githubAvatarURL(ctx, repoPath)
	if url == "" {
		return nil, "", nil
	}
	dlCtx, cancel := context.WithTimeout(ctx, githubAvatarFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, "", nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.WarnContext(ctx, "repos: close the avatar response", "err", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, icons.MaxBytes+1))
	if err != nil {
		return nil, "", nil
	}
	if len(data) > icons.MaxBytes {
		return nil, "", nil
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}

func githubAvatarURL(
	ctx context.Context,
	repoPath string,
) string {
	//nolint:gosec // G204: fixed git subcommand; repoPath is a daemon-managed repo path, not shell-interpreted or attacker-controlled.
	raw, err := exec.CommandContext(ctx, binpath.Git(), "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	slug, err := githubSlugFromURL(strings.TrimSpace(string(raw)))
	if err != nil {
		return ""
	}
	//nolint:gosec // G204: gh invoked with fixed args; slug is parsed from the repo's own git remote URL and passed as a discrete argv entry, not shell-interpreted.
	out, err := exec.CommandContext(ctx, binpath.Resolve("gh"), "api", "repos/"+slug, "--jq", ".owner.avatar_url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func githubSlugFromURL(
	rawURL string,
) (string, error) {
	rawURL = strings.TrimSuffix(strings.TrimSpace(rawURL), ".git")
	if strings.HasPrefix(rawURL, "git@") {
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
	}
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		path := rawURL[idx+3:]
		if slash := strings.Index(path, "/"); slash >= 0 {
			return path[slash+1:], nil
		}
	}
	return "", fmt.Errorf("unrecognised URL: %q", rawURL)
}

const githubAvatarFetchTimeout = 10 * time.Second
