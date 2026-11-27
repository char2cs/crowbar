package avatar

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

const paletteSize = 8

func palette() []string {
	return []string{
		"avatar-rose",
		"avatar-amber",
		"avatar-emerald",
		"avatar-cyan",
		"avatar-indigo",
		"avatar-violet",
		"avatar-slate",
		"avatar-pink",
	}
}

func Label(
	name string,
) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return "?"
}

func Color(
	name string,
) string {
	p := palette()
	h := fnv.New32a()
	if _, err := h.Write([]byte(name)); err != nil {
		return p[0]
	}
	return p[h.Sum32()%paletteSize]
}

var iconCandidates = []string{
	"favicon.svg",
	"favicon.ico",
	"favicon.png",
	"logo.svg",
	"logo.png",
	"public/logo.svg",
	"public/logo.png",
	"public/favicon.svg",
	"public/favicon.ico",
	"public/favicon.png",
	"src/assets/logo.svg",
	"src/assets/logo.png",
}

func ScanRepoIcon(repoPath string) string {
	for _, rel := range iconCandidates {
		abs := filepath.Join(repoPath, rel)
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return ""
}

const avatarFetchTimeout = 10 * time.Second

const maxAvatarBytes = 2 << 20

func FetchOwnerAvatarBytes(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error) {
	url := ownerAvatarURL(ctx, repoPath)
	if url == "" {
		return nil, "", nil
	}
	return downloadBytes(ctx, url)
}

func ownerAvatarURL(
	ctx context.Context,
	repoPath string,
) string {
	//nolint:gosec // G204: fixed git subcommand; repoPath is an internal repo root, not user-controlled shell input.
	raw, err := exec.CommandContext(ctx, binpath.Git(), "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	slug, err := githubSlug(strings.TrimSpace(string(raw)))
	if err != nil {
		return ""
	}
	//nolint:gosec // G204: fixed gh subcommand; "gh" is resolved to a trusted install path and slug is the parsed owner/repo, not shell input.
	out, err := exec.CommandContext(ctx, binpath.Resolve("gh"), "api", "repos/"+slug, "--jq", ".owner.avatar_url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func downloadBytes(
	ctx context.Context,
	url string,
) ([]byte, string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, avatarFetchTimeout)
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
			slog.WarnContext(ctx, "avatar: close the download", "err", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes+1))
	if err != nil {
		return nil, "", nil
	}
	if len(data) > maxAvatarBytes {
		return nil, "", nil
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}

func githubSlug(
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
	return "", fmt.Errorf("avatar: unrecognised remote URL: %q", rawURL)
}
