//go:build !windows

package metadata

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// resolveHome expands the home template into an absolute path.
// If os.UserHomeDir fails (e.g. HOME unset in a container), it logs a warning
// and returns the template raw, producing a path relative to the working directory.
func resolveHome(tmpl string) string {
	// Replace {{home}} with "~" equivalent then expand.
	// The metadata.yaml home template is "{{home}}/.crowbar", so first resolve
	// the {{home}} placeholder to the actual OS home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn(
			"cannot determine user home directory; crowbar files will be written to the raw template path",
			"template", tmpl,
		)
		return tmpl
	}

	// Substitute {{home}} with the OS home, then normalize separators.
	expanded := strings.ReplaceAll(tmpl, "{{home}}", home)
	return filepath.FromSlash(expanded)
}
