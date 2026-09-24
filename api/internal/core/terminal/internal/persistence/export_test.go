package persistence

import (
	"os"
	"path/filepath"
)

// ReadBuf returns the contents of <dir>/<sessionID>.buf, or nil when there is
// none. Production restores scrollback through the session snapshot; only the
// tests read the raw file back.
func ReadBuf(dir, sessionID string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, sessionID+".buf")) //nolint:gosec // G304: test-controlled temp dir
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}
