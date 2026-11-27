package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var ErrPathEscapesWorkspace = problem.New(problem.InvalidArgument, "path escapes the workspace")

var ErrNotAbsolute = problem.New(problem.InvalidArgument, "path is not absolute")

var ErrDanglingSymlink = problem.New(problem.InvalidArgument, "symlink target does not exist")

var ErrFileTooLarge = problem.New(problem.TooLarge, "file too large")

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
