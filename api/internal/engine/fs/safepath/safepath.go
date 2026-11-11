// Package safepath is the single containment validator for every fs operation taking a user-supplied
// workspace-relative path: it rejects absolute paths and escapes via ".." or symlinks with
// ErrPathEscapesWorkspace, so no op can read, write or delete outside the workspace.
package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrPathEscapesWorkspace is returned when a workspace-relative path is absolute, traverses above the
// root, or resolves via a symlink outside it; it maps to HTTP 400.
var ErrPathEscapesWorkspace = problem.New(problem.InvalidArgument, "path escapes the workspace")

// ErrNotAbsolute is returned by the host-path reader and writer for a path that
// is not absolute and clean: only a path the usecase resolved reaches the disk.
var ErrNotAbsolute = problem.New(problem.InvalidArgument, "path is not absolute")

// ErrDanglingSymlink is returned for a write through a symlink whose target does
// not exist: creating it could put a file anywhere the link points.
var ErrDanglingSymlink = problem.New(problem.InvalidArgument, "symlink target does not exist")

// ErrFileTooLarge is returned by the content reader when a file exceeds the
// maxReadBytes cap (25 MiB). The api libs error mapper maps it to HTTP 413
// Request Entity Too Large.
var ErrFileTooLarge = problem.New(problem.TooLarge, "file too large")

// Resolve validates rel is a safe path inside root and returns filepath.Join(root, rel). Lexically rel
// must satisfy filepath.IsLocal ("" is the root). Symlinks are checked on the deepest existing ancestor,
// since the target may not exist yet; if canonicalizing fails, the lexical guarantee stands.
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

// Canonical is the one spelling of an absolute path: symlinks in its deepest
// existing ancestor resolved, the not-yet-existing rest appended, so a file has
// the same identity before and after it is created.
func Canonical(
	path string,
) string {
	clean := filepath.Clean(path)
	ancestor := existingAncestor(clean)
	if ancestor == "" {
		return clean
	}
	canon, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return clean
	}
	return filepath.Join(canon, strings.TrimPrefix(clean, ancestor))
}

// Locate walks path's ancestors deepest-first to the first that is, by file identity (through symlinks,
// firmlinks and case), the directory of one or more roots, returning their indexes and the rest of path
// below it as the directory spells it. A nonexistent root matches nothing.
func Locate(
	path string,
	roots []string,
) ([]int, string) {
	infos := statAll(roots)
	dir := filepath.Clean(path)
	var below []string
	for {
		if hits := sameAs(dir, infos); len(hits) > 0 {
			return hits, spell(dir, below)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ""
		}
		below = append([]string{filepath.Base(dir)}, below...)
		dir = parent
	}
}

func statAll(
	paths []string,
) []os.FileInfo {
	infos := make([]os.FileInfo, len(paths))
	for i, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			infos[i] = fi
		}
	}
	return infos
}

// sameAs answers the indexes of the infos that are dir itself.
func sameAs(
	dir string,
	infos []os.FileInfo,
) []int {
	fi, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	var hits []int
	for i, info := range infos {
		if info != nil && os.SameFile(fi, info) {
			hits = append(hits, i)
		}
	}
	return hits
}

// spell joins names under dir, each as dir's own entry spells it.
func spell(
	dir string,
	names []string,
) string {
	spelled := make([]string, len(names))
	for i, name := range names {
		spelled[i] = entryName(dir, name)
		dir = filepath.Join(dir, spelled[i])
	}
	return filepath.Join(spelled...)
}

// entryName is the name of dir's entry that name reaches on a case- or
// normalization-insensitive volume, else name. Case is tried first; any other
// alias, such as NFC typed for an NFD entry, is matched by identity.
func entryName(
	dir string,
	name string,
) string {
	want, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return name
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return name
	}
	for _, e := range entries {
		if e.Name() == name {
			return name
		}
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) && sameEntry(filepath.Join(dir, e.Name()), want) {
			return e.Name()
		}
	}
	for _, e := range entries {
		if sameEntry(filepath.Join(dir, e.Name()), want) {
			return e.Name()
		}
	}
	return name
}

func sameEntry(
	path string,
	want os.FileInfo,
) bool {
	got, err := os.Lstat(path)
	return err == nil && os.SameFile(got, want)
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
