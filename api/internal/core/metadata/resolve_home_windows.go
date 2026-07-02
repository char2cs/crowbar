//go:build windows

package metadata

import "os"

func resolveHome() string {
	if override := os.Getenv(HomeEnvVar); override != "" {
		return override
	}
	home := Get().Paths.Home.Resolve()
	return os.ExpandEnv(home)
}
