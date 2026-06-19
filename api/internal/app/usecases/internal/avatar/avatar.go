// Package avatar generates deterministic avatar labels and colors for repos from
// their names (00 §5.7). Pure functions — no state.
package avatar

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// paletteSize is the number of entries in palette(); must equal len(palette()).
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

// Palette returns the avatar color token set.
func Palette() []string {
	return palette()
}

// Label returns the single-char avatar badge: first alphanumeric char of name,
// uppercased; "?" when none.
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

// Color returns a stable palette token hashed from name.
func Color(
	name string,
) string {
	p := palette()
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return p[h.Sum32()%paletteSize]
}

// iconCandidates is the ordered list of relative paths checked when scanning
// a repo root for an icon file. First match wins.
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

// ScanRepoIcon walks iconCandidates relative to repoPath and returns the
// absolute path of the first file found, or "" when none match.
func ScanRepoIcon(repoPath string) string {
	for _, rel := range iconCandidates {
		abs := filepath.Join(repoPath, rel)
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return ""
}

// avatarFetchTimeout bounds the GitHub owner-avatar download so a slow or
// unreachable host never stalls the import path.
const avatarFetchTimeout = 10 * time.Second

// FetchOwnerAvatarBytes resolves the repo owner's avatar URL via the gh CLI
// and downloads its bytes. It is a best-effort helper: when there is no origin
// remote, no gh auth, or the download fails, it returns (nil, "", nil) so the
// caller falls back to a generated avatar without surfacing an error. Genuine
// programming/transport errors are not swallowed beyond the soft-failure cases.
//
// Returns the raw image bytes and the response content-type.
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

// ownerAvatarURL shells out to git + gh to resolve the owner avatar URL for the
// repo at repoPath, returning "" on any soft failure.
func ownerAvatarURL(
	ctx context.Context,
	repoPath string,
) string {
	raw, err := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	slug, err := githubSlug(strings.TrimSpace(string(raw)))
	if err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, "gh", "api", "repos/"+slug, "--jq", ".owner.avatar_url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// downloadBytes fetches url and returns its body bytes + content-type, or
// (nil, "", nil) on any transport failure (best-effort).
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", nil
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}

// githubSlug extracts "owner/repo" from a GitHub remote URL (SSH or HTTPS).
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
