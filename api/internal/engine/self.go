package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// selfExecutable is the binary this daemon runs from, for the CLIs it spawns to
// run `crowbar mcp`. The relay is version-agnostic, so a binary replaced in place
// still serves; a deleted one is still reachable on Linux through /proc.
func selfExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("engine: own executable: %w", err)
	}
	return liveExecutable(exe, os.Getpid()), nil
}

// liveExecutable is exe resolved through symlinks, or, on Linux, the process's
// /proc entry once exe is gone: it stays executable while the process lives.
func liveExecutable(
	exe string,
	pid int,
) string {
	trimmed := strings.TrimSuffix(exe, " (deleted)")
	if resolved, err := filepath.EvalSymlinks(trimmed); err == nil {
		return resolved
	}
	if runtime.GOOS == "linux" {
		return fmt.Sprintf("/proc/%d/exe", pid)
	}
	return trimmed
}
