package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// errLayoutUnreadable is returned when the LAYOUT file exists but cannot be
// read or does not hold a non-negative integer.
var errLayoutUnreadable = errors.New("migrate: LAYOUT file is unreadable")

var layoutNumber = regexp.MustCompile(`^[0-9]+$`)

// readLayout returns the home's layout version: 0 when the file is absent (a
// legacy home), an error wrapping errLayoutUnreadable for anything else that is
// not a bare non-negative integer.
func readLayout(
	h home.Home,
) (int, error) {
	path := h.LayoutFile()
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is the home's own LAYOUT file, not user input
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", errLayoutUnreadable, path, err)
	}
	text := strings.TrimSpace(string(raw))
	if !layoutNumber.MatchString(text) {
		return 0, fmt.Errorf("%w: %s: not a non-negative integer", errLayoutUnreadable, path)
	}
	layout, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", errLayoutUnreadable, path, err)
	}
	return layout, nil
}

// writeLayout durably replaces the LAYOUT file with v: a synced temp file in the
// same directory is renamed over it, so a crash leaves the old or the new value.
// The home must already exist, and the caller must hold the home lock.
func writeLayout(
	h home.Home,
	v int,
) error {
	if v < 0 {
		return fmt.Errorf("migrate: write LAYOUT: negative layout %d", v)
	}
	path := h.LayoutFile()
	tmp := path + ".tmp"
	if err := stageLayout(tmp, v); err != nil {
		return errors.Join(err, removeStaged(tmp))
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(fmt.Errorf("migrate: write LAYOUT: commit: %w", err), removeStaged(tmp))
	}
	return syncDir(h.Root())
}

// stageLayout durably writes v, as LAYOUT holds it, to the file at path.
func stageLayout(
	path string,
	v int,
) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // G304: path is a LAYOUT file of the home's own, not user input
	if err != nil {
		return fmt.Errorf("migrate: write layout: create %s: %w", path, err)
	}
	if _, err := file.WriteString(strconv.Itoa(v) + "\n"); err != nil {
		return errors.Join(fmt.Errorf("migrate: write layout: write %s: %w", path, err), file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(fmt.Errorf("migrate: write layout: sync %s: %w", path, err), file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("migrate: write layout: close %s: %w", path, err)
	}
	return nil
}

func removeStaged(
	path string,
) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("migrate: remove %s: %w", path, err)
}

// syncDir makes a rename or file creation inside dir durable. Windows cannot
// flush a directory handle, so it is skipped there.
func syncDir(
	dir string,
) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(dir) //nolint:gosec // G304: dir is the Crowbar home, not user input
	if err != nil {
		return fmt.Errorf("migrate: sync %s: %w", dir, err)
	}
	if err := handle.Sync(); err != nil {
		return errors.Join(fmt.Errorf("migrate: sync %s: %w", dir, err), handle.Close())
	}
	if err := handle.Close(); err != nil {
		return fmt.Errorf("migrate: sync %s: close: %w", dir, err)
	}
	return nil
}
