// Package safepath is the single containment validator shared by every fs
// engine operation that accepts a user-supplied, workspace-relative path
// (read, write, create, rename, delete, tree). It rejects absolute paths and
// any path that escapes the workspace root via ".." traversal or a symlink
// pointing outside the root, returning ErrPathEscapesWorkspace so the handler
// layer maps it to a 4xx (see internal/api/libs.StatusAndMessage).
//
// Security finding R1: before this package, fs ops did a bare
// filepath.Join(root, rel) with no containment check, so "../" or absolute
// paths escaped the workspace (arbitrary host read/write/delete, and RCE via
// writing an executable ../.git/hooks/pre-commit). Every op now resolves its
// relative path through Resolve before touching the filesystem.
package safepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPathEscapesWorkspace is returned when a workspace-relative path is
// absolute, traverses above the workspace root, or resolves (via a symlink)
// to a location outside the root. The api libs error mapper maps it to HTTP
// 400 Bad Request.
var ErrPathEscapesWorkspace = errors.New("path escapes the workspace")

// ErrFileTooLarge is returned by the content reader when a file exceeds the
// maxReadBytes cap (25 MiB). The api libs error mapper maps it to HTTP 413
// Request Entity Too Large (hardening finding R16).
var ErrFileTooLarge = errors.New("file too large")

// Resolve validates that rel is a safe path inside root and returns the
// cleaned absolute path (filepath.Join(root, rel)) to operate on. rel must be
// a workspace-relative path; an absolute rel, a ".." escape, or a symlinked
// escape all yield ErrPathEscapesWorkspace.
//
// Lexical containment (always enforced):
//   - rel must not be absolute.
//   - filepath.IsLocal(rel) must hold: this rejects "..", any path that
//     escapes the root after cleaning (e.g. "a/../../b"), absolute paths, and
//     (on Windows) reserved/volume-rooted names. It is the standard library's
//     containment predicate for exactly this class. An empty rel is allowed
//     and denotes the workspace root itself (used by the tree walker).
//
// Symlink containment (best-effort, defends the symlink-escape vector):
//
//	The target may not exist yet (creating a new file/dir), so we cannot
//	EvalSymlinks the full path unconditionally — that would fail for every
//	create. Instead we EvalSymlinks the deepest ANCESTOR that already exists
//	(the target itself if present, else the nearest existing parent) and the
//	root, then require the canonical ancestor to remain within the canonical
//	root. This catches a symlinked parent directory pointing outside the
//	workspace while still permitting new files whose parents are honest. If
//	either path cannot be canonicalized for an unrelated reason, we fall back
//	to the lexical guarantee already established above.
func Resolve(
	root string,
	rel string,
) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q is absolute", ErrPathEscapesWorkspace, rel)
	}

	if rel != "" && !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: %q traverses outside the root", ErrPathEscapesWorkspace, rel)
	}

	full := filepath.Join(root, rel)

	if err := checkSymlinkContainment(root, full); err != nil {
		return "", err
	}

	return full, nil
}

func checkSymlinkContainment(
	root string,
	full string,
) error {
	canonRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil //nolint:nilerr // lexical containment already enforced by Resolve
	}

	ancestor := existingAncestor(full)
	if ancestor == "" {
		return nil
	}

	canonAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return nil //nolint:nilerr // lexical containment already enforced by Resolve
	}

	if !withinRoot(canonRoot, canonAncestor) {
		return fmt.Errorf("%w: %q resolves outside the root via a symlink", ErrPathEscapesWorkspace, full)
	}

	return nil
}

func existingAncestor(
	full string,
) string {
	p := filepath.Clean(full)
	for {
		if _, err := os.Lstat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
}

func withinRoot(
	root string,
	canonPath string,
) bool {
	rel, err := filepath.Rel(root, canonPath)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
