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

var ErrLocked = errors.New("workspace is locked")

func ApplyToFile(
	absPath string,
	re *regexp.Regexp,
	replacement string,
	locked bool,
) error {
	if locked {
		return ErrLocked
	}

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
		return nil
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
