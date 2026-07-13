// Package binpath resolves external CLI binaries robustly. A daemon launched
// by macOS launchd (the packaged .app) inherits a minimal PATH
// (/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin) that misses Homebrew, so a
// bare exec of tools like gh fails even when they are installed. Resolve falls
// back to well-known install locations after the PATH lookup.
package binpath

import (
	"os"
	"os/exec"
	"path/filepath"
)

// wellKnownDirs are extra directories probed when the PATH lookup fails, in
// priority order.
//
// ~/.local/bin is first and is NOT reachable by a PATH lookup alone: the daemon
// repairs launchd's minimal PATH from a LOGIN NON-INTERACTIVE shell (core/shellenv),
// which sources .zshenv/.zprofile but never .zshrc — and .zshrc is where that dir is
// conventionally put on PATH. It is also where the agentic CLIs (claude, codex)
// install by default, so without it the packaged .app cannot spawn one at all.
func wellKnownDirs() []string {
	dirs := make([]string, 0, 4)
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin")) // claude/codex default install
	}
	return append(dirs,
		"/opt/homebrew/bin", // macOS arm64 Homebrew
		"/usr/local/bin",    // macOS intel Homebrew / manual installs
		"/usr/bin",
	)
}

// Resolve returns the executable path for name: the PATH lookup first, then
// the well-known dirs. When nothing is found it returns name unchanged so the
// subsequent exec fails with the usual "executable not found", preserving the
// call site's soft-fail semantics.
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
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return name
}
