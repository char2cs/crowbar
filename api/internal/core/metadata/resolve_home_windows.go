//go:build windows

package metadata

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveHome expands the home template into an absolute path on Windows.
// Uses os.UserHomeDir() to obtain the current user's home directory.
func resolveHome(tmpl string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return tmpl
	}
	expanded := strings.ReplaceAll(tmpl, "{{home}}", home)
	return filepath.FromSlash(expanded)
}
