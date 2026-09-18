//go:build windows

package metadata

import "os"

func resolveHome() string {
	if override := os.Getenv(HomeEnvVar); override != "" {
		return override
	}
	return defaultHome()
}

func defaultHome() string {
	home := Get().Paths.Home.Resolve()
	return os.ExpandEnv(home)
}
