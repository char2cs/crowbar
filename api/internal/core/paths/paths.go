// Package paths resolves named Crowbar directories from metadata, creates them
// on demand with a per-path mutex, and returns their absolute paths.
package paths

import (
	"fmt"
	"os"
	"sync"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

var mu sync.Map

func ensure(
	path string,
) (string, error) {
	v, _ := mu.LoadOrStore(path, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	defer m.Unlock()
	if err := os.MkdirAll(path, 0o750); err != nil {
		return "", fmt.Errorf("paths: create %q: %w", path, err)
	}
	return path, nil
}

// EventsAt returns the event-store directory rooted at homeDir, creating it if absent.
func EventsAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetEventsPathAt(homeDir))
}

// StoreAt returns the GORM read-model directory rooted at homeDir, creating it if absent.
func StoreAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetStorePathAt(homeDir))
}
