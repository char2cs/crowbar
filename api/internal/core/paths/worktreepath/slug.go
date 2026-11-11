package worktreepath

import (
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RemoteSlug returns a repository's on-disk identity host/owner/repo from its remote URL (SSH and URL
// forms agree). Without such a remote it falls back to the immutable PathSlug, and only for rows predating
// PathSlug to the renameable Name, since a slug that moved would strand derived worktrees.
func RemoteSlug(
	repo domain.Repository,
) string {
	if slug, ok := slugFromRemoteURL(repo.RemoteURL); ok {
		return slug
	}
	if repo.PathSlug != "" {
		return repo.PathSlug
	}
	return repo.Name
}

// SeedPathSlug returns the PathSlug to persist at import: the remote slug, else the cleaned path's base
// name — never the display name, so the two can diverge. An unusable leaf (e.g. "..") yields "", which
// falls through to the display name rather than collapsing a level out of the layout.
func SeedPathSlug(
	remoteURL string,
	repoPath string,
) string {
	if slug, ok := slugFromRemoteURL(remoteURL); ok {
		return slug
	}
	leaf := filepath.Base(filepath.Clean(repoPath))
	if strings.ContainsAny(leaf, `/\`) || strings.Trim(leaf, ".") == "" {
		return ""
	}
	return leaf
}

func slugFromRemoteURL(
	remoteURL string,
) (string, bool) {
	url := strings.TrimSuffix(strings.TrimSpace(remoteURL), ".git")
	if url == "" {
		return "", false
	}
	authority, path, ok := splitRemote(url)
	if !ok {
		return "", false
	}
	host := hostFromAuthority(authority)
	path = strings.Trim(path, "/")
	if host == "" || path == "" {
		return "", false
	}
	return host + "/" + path, true
}

func splitRemote(
	url string,
) (string, string, bool) {
	if idx := strings.Index(url, "://"); idx >= 0 {
		return strings.Cut(url[idx+3:], "/")
	}
	return strings.Cut(url, ":")
}

func hostFromAuthority(
	authority string,
) string {
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		authority = authority[at+1:]
	}
	if colon := strings.Index(authority, ":"); colon >= 0 {
		authority = authority[:colon]
	}
	return authority
}
