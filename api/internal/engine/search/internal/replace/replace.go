// Package replace applies regexp-based replacements to files atomically.
package replace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/match"
)

// ErrLocked is returned when a replace is attempted on a locked workspace.
var ErrLocked = errors.New("workspace is locked")

// ApplyToFile replaces all matches of re in the file at absPath with
// replacement. The write is atomic: content is written to a temp file and then
// renamed into place. Binary files are silently skipped.
//
// Returns ErrLocked if locked is true.
func ApplyToFile(
	absPath string,
	re *regexp.Regexp,
	replacement string,
	locked bool,
) error {
	if locked {
		return ErrLocked
	}

	// Stat before anything else so we can restore permissions later.
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("replace: stat %s: %w", absPath, err)
	}
	originalMode := info.Mode()

	binary, err := sniffBinary(absPath)
	if err != nil {
		return err
	}
	if binary {
		return nil // skip binary files silently
	}

	data, err := os.ReadFile(absPath) //nolint:gosec // path is controlled by the caller
	if err != nil {
		return fmt.Errorf("replace: read %s: %w", absPath, err)
	}

	updated := re.ReplaceAllString(string(data), replacement)

	if err := writeAtomic(absPath, updated, originalMode); err != nil {
		return fmt.Errorf("replace: write %s: %w", absPath, err)
	}

	return nil
}

// sniffBinary reports whether the first 8 KB of absPath look binary.
func sniffBinary(
	absPath string,
) (binary bool, err error) {
	f, err := os.Open(absPath) //nolint:gosec // path is controlled by the caller
	if err != nil {
		return false, fmt.Errorf("replace: open for sniff %s: %w", absPath, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("replace: close %s: %w", absPath, closeErr))
		}
	}()
	sample := make([]byte, 8192)
	n, readErr := f.Read(sample)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, fmt.Errorf("replace: sniff %s: %w", absPath, readErr)
	}
	return match.IsBinary(sample[:n]), nil
}

// writeAtomic writes content to a sibling temp file, sets its permissions to
// match originalMode, then renames it over dst. A failure removes the temp, and
// a removal that fails too is reported with it.
func writeAtomic(
	dst string,
	content string,
	originalMode os.FileMode,
) (err error) {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".crowbar-replace-*")
	if err != nil {
		return fmt.Errorf("replace: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmpName))
		}
	}()

	if _, err := tmp.WriteString(content); err != nil {
		return errors.Join(fmt.Errorf("replace: write temp: %w", err), tmp.Close())
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("replace: close temp: %w", err)
	}

	if err := os.Chmod(tmpName, originalMode); err != nil {
		return fmt.Errorf("replace: chmod %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("replace: rename: %w", err)
	}

	return nil
}
