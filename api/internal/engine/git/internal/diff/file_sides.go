package diff

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// MaxSideBytes bounds one side of a file read for context expansion. A file past
// it is refused rather than truncated: a half file would shift every line number
// below the cut.
const MaxSideBytes = 2 << 20

// ErrFileTooLarge reports that a side exceeds MaxSideBytes.
var ErrFileTooLarge = errors.New("diff: file side too large")

// ErrNonLocalPath rejects a path that is absolute or climbs out of the repository.
var ErrNonLocalPath = errors.New("diff: path leaves the repository")

// ErrEmptyFilePath rejects a read that names no file.
var ErrEmptyFilePath = errors.New("diff: file sides: path is required")

// FileSides reads the old and new text of one file for the diff `git diff <ref>`
// would produce. The old side is the file at the base of ref; the new side is
// the working tree for a single revision, or the right-hand commit for an
// `<a>..<b>` range.
//
// oldPath names the file on the old side (a rename source) and defaults to path.
func FileSides(
	ctx context.Context,
	repoPath string,
	ref string,
	path string,
	oldPath string,
) (gitdomain.ReviewFileSides, error) {
	if path == "" {
		return gitdomain.ReviewFileSides{}, ErrEmptyFilePath
	}
	if oldPath == "" {
		oldPath = path
	}
	for _, p := range []string{path, oldPath} {
		if !filepath.IsLocal(filepath.FromSlash(p)) {
			return gitdomain.ReviewFileSides{}, fmt.Errorf("diff: file sides: %w: %q", ErrNonLocalPath, p)
		}
	}
	base, head, ranged := strings.Cut(ref, "..")
	oldSide, err := readRevisionSide(ctx, repoPath, base, oldPath)
	if err != nil {
		return gitdomain.ReviewFileSides{}, fmt.Errorf("diff: file sides: old: %w", err)
	}
	var newSide *string
	if ranged {
		newSide, err = readRevisionSide(ctx, repoPath, head, path)
	} else {
		newSide, err = readWorktreeSide(repoPath, path)
	}
	if err != nil {
		return gitdomain.ReviewFileSides{}, fmt.Errorf("diff: file sides: new: %w", err)
	}
	return gitdomain.ReviewFileSides{Old: oldSide, New: newSide}, nil
}

// readRevisionSide returns the blob at rev:path, or nil when rev has no such
// file. The size is asked first so an oversized blob is never read into memory.
func readRevisionSide(
	ctx context.Context,
	repoPath string,
	rev string,
	path string,
) (*string, error) {
	spec := rev + ":" + path
	probe := exec.Git(ctx, repoPath, "cat-file", "-s", spec)
	if probe.ExitCode != 0 {
		return nil, nil //nolint:nilnil // an absent side is a value, not a failure
	}
	size, err := strconv.ParseInt(strings.TrimSpace(probe.Stdout), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse blob size %q: %w", probe.Stdout, err)
	}
	if size > MaxSideBytes {
		return nil, ErrFileTooLarge
	}
	r := exec.Git(ctx, repoPath, "cat-file", "blob", spec)
	if err := exec.RequireSuccess("diff: read blob", r); err != nil {
		return nil, err
	}
	return &r.Stdout, nil
}

// readWorktreeSide returns the working-tree file at path, or nil when it is
// absent. A symlink is refused: the diff shows a link as its target string, and
// following one would read outside the repository.
func readWorktreeSide(
	repoPath string,
	path string,
) (*string, error) {
	full := filepath.Join(repoPath, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // an absent side is a value, not a failure
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path %q is not a regular file", path)
	}
	if info.Size() > MaxSideBytes {
		return nil, ErrFileTooLarge
	}
	data, err := os.ReadFile(full) //nolint:gosec // G304: FileSides checked the path is repo-local and Lstat refused symlinks
	if err != nil {
		return nil, err
	}
	text := string(data)
	return &text, nil
}
