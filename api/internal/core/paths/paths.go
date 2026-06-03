// Package paths resolves named Crowbar directories from metadata, creates them
// on demand with a per-path mutex, and returns their absolute paths.
//
// Config (config.yaml) is intentionally absent — it is a file, not a directory,
// and is owned by the config package.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// mu stores one *sync.Mutex per absolute path, created on first access.
// Serializes concurrent directory creation for the same path.
var mu sync.Map

// ensure creates the directory at path if it does not exist and returns path.
// Concurrent calls for the same path are serialized by a per-path mutex.
func ensure(path string) (string, error) {
	v, _ := mu.LoadOrStore(path, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	defer m.Unlock()
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("paths: create %q: %w", path, err)
	}
	return path, nil
}

// Events returns the absolute path to the event-store directory,
// creating it if it does not exist. (~/.crowbar/state/events/)
func Events() (string, error) {
	return ensure(filepath.Join(metadata.GetHome(), "state", "events"))
}

// EventsAt returns the absolute path to the event-store directory rooted at
// homeDir instead of the process-level home, creating it if it does not exist.
func EventsAt(homeDir string) (string, error) {
	return ensure(filepath.Join(metadata.GetHomeAt(homeDir), "state", "events"))
}

// Store returns the absolute path to the read-model store directory,
// creating it if it does not exist. (~/.crowbar/state/store/)
func Store() (string, error) {
	return ensure(filepath.Join(metadata.GetHome(), "state", "store"))
}

// StoreAt returns the absolute path to the read-model store directory rooted
// at homeDir instead of the process-level home, creating it if it does not exist.
func StoreAt(homeDir string) (string, error) {
	return ensure(filepath.Join(metadata.GetHomeAt(homeDir), "state", "store"))
}

// Runs returns the absolute path to the agent-run artifacts directory,
// creating it if it does not exist. (~/.crowbar/runs/)
func Runs() (string, error) {
	return ensure(filepath.Join(metadata.GetHome(), "runs"))
}

// RunsAt returns the absolute path to the agent-run artifacts directory rooted
// at homeDir instead of the process-level home, creating it if it does not exist.
func RunsAt(homeDir string) (string, error) {
	return ensure(filepath.Join(metadata.GetHomeAt(homeDir), "runs"))
}

// Logs returns the absolute path to the logs directory,
// creating it if it does not exist. (~/.crowbar/logs/)
func Logs() (string, error) {
	return ensure(filepath.Join(metadata.GetHome(), "logs"))
}

// LogsAt returns the absolute path to the logs directory rooted at homeDir
// instead of the process-level home, creating it if it does not exist.
func LogsAt(homeDir string) (string, error) {
	return ensure(filepath.Join(metadata.GetHomeAt(homeDir), "logs"))
}
