package worktreepath

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RemoteSlug returns a repository's on-disk identity slug host/owner/repo,
// parsed from its git remote URL.
//
// Both SSH (git@github.com:char2cs/crowbar.git) and URL
// (https://github.com/char2cs/crowbar) remote forms resolve to the same
// host/owner/repo slug, so the repo's globally-unique remote identity is encoded
// in full on disk (spec §3.9). When the repository has no remote, the slug falls
// back to the repository name as a single-leaf identity (the no-remote case,
// spec §3.9).
func RemoteSlug(
	repo domain.Repository,
) string {
	if slug, ok := slugFromRemoteURL(repo.RemoteURL); ok {
		return slug
	}
	return repo.Name
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
