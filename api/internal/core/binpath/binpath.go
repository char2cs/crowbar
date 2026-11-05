// Package binpath resolves external CLI binaries robustly. A daemon launched by
// launchd inherits a minimal PATH that misses Homebrew, so Resolve falls back to
// well-known install locations after the PATH lookup.
package binpath

import (
	"os"
	"os/exec"
	"path/filepath"
)

// wellKnownDirs are extra directories probed, in priority order, when the PATH lookup
// fails. ~/.local/bin comes first: vendor CLIs install there, and it is usually added
// in .zshrc, which the login non-interactive shell behind core/shellenv never sources.
func wellKnownDirs() []string {
	dirs := make([]string, 0, 4)
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin")) // user-local installs
	}
	return append(dirs,
		"/opt/homebrew/bin", // macOS arm64 Homebrew
		"/usr/local/bin",    // macOS intel Homebrew / manual installs
		"/usr/bin",
	)
}

// Resolve returns the executable path for name: the PATH lookup first, then the
// well-known dirs. When nothing is found it returns name unchanged so the exec
// fails with the usual "executable not found".
func Resolve(
	name string,
) string {
	return resolve(name, wellKnownDirs())
}

func resolve(
	name string,
	dirs []string,
) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range dirs {
		candidate := filepath.Join(dir, name)
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return name
}

func isExecutableFile(
	path string,
) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
