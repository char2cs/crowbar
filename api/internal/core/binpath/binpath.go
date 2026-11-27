package binpath

import (
	"os"
	"os/exec"
	"path/filepath"
)

func wellKnownDirs() []string {
	dirs := make([]string, 0, 4)
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return append(dirs,
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
	)
}

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
