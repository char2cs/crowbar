//go:build !windows

package metadata

import (
	"os"
	"path/filepath"
	"strings"
)

func resolveHome() string {
	if override := os.Getenv(HomeEnvVar); override != "" {
		return override
	}
	home := Get().Paths.Home.Resolve()
	if !strings.HasPrefix(home, "~") {
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return home
	}
	return filepath.Join(userHome, strings.TrimPrefix(home, "~"))
}
