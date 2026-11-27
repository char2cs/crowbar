package persistence

import (
	"os"
	"path/filepath"
)

func ReadBuf(dir, sessionID string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, sessionID+".buf"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}
