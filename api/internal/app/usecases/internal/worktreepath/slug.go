package worktreepath

import (
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RemoteSlug returns a repository's on-disk identity slug host/owner/repo,
// parsed from its git remote URL.
//
// Both SSH (git@github.com:char2cs/crowbar.git) and URL
// (https://github.com/char2cs/crowbar) remote forms resolve to the same
// host/owner/repo slug, so the repo's globally-unique remote identity is encoded
// in full on disk (spec §3.9).
//
// A repository whose remote does not encode a host/owner/repo identity (no
// remote at all, or a local bare path) falls back to its immutable
// Repository.PathSlug, the single-leaf identity seeded from the repo's own path
// at import (the no-remote case, spec §3.9). The display Name is the LAST
// resort, reached only by rows written before PathSlug existed: it is
// user-renameable, and a slug that moved with a rename would strand every
// already-derived worktree under the previous slug.
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

// SeedPathSlug returns the on-disk identity to persist as
// Repository.PathSlug for a repository being imported: the remote slug when the
// remote URL encodes a host/owner/repo identity, otherwise the repo directory's
// own base name.
//
// It is seeded from the PATH, never from the user-supplied display name, so the
// name and the on-disk layout can diverge freely afterwards.
//
// The path is CLEANED before its leaf is taken, and a leaf that is not a usable
// directory name yields "". The import path is only stat'd, never normalised, so
// ".../widget/.." arrives verbatim and its raw base is ".." — which Derive would
// then quietly collapse a level out of the layout instead of rejecting (it stays
// under crowbar home, so the escape guard never fires). "" is the safe answer:
// the slug chain falls through to the display name, which the create and rename
// endpoints already refuse to accept in that shape.
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
