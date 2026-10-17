//go:build windows

package home

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultRoot() (string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home: default root: %w", err)
	}
	return filepath.Join(userHome, "Documents", ".crowbar"), nil
}
