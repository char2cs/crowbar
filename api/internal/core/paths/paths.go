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

// State returns the state directory (parent of events and store), creating it if absent.
func State() (string, error) {
	return ensure(metadata.GetStateDirPath())
}

// StateAt returns the state directory rooted at homeDir, creating it if absent.
func StateAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetStateDirPathAt(homeDir))
}

// Events returns the event-store directory, creating it if absent.
func Events() (string, error) {
	return ensure(metadata.GetEventsPath())
}

// EventsAt returns the event-store directory rooted at homeDir, creating it if absent.
func EventsAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetEventsPathAt(homeDir))
}

// Store returns the GORM read-model directory, creating it if absent.
func Store() (string, error) {
	return ensure(metadata.GetStorePath())
}

// StoreAt returns the GORM read-model directory rooted at homeDir, creating it if absent.
func StoreAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetStorePathAt(homeDir))
}

// Runs returns the agent-run artifacts directory, creating it if absent.
func Runs() (string, error) {
	return ensure(metadata.GetRunsPath())
}

// RunsAt returns the agent-run artifacts directory rooted at homeDir, creating it if absent.
func RunsAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetRunsPathAt(homeDir))
}

// Logs returns the logs directory, creating it if absent.
func Logs() (string, error) {
	return ensure(metadata.GetLogsPath())
}

// LogsAt returns the logs directory rooted at homeDir, creating it if absent.
func LogsAt(
	homeDir string,
) (string, error) {
	return ensure(metadata.GetLogsPathAt(homeDir))
}
