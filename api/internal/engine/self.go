package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func selfExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("engine: own executable: %w", err)
	}
	return liveExecutable(exe, os.Getpid()), nil
}

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
