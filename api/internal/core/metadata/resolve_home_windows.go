//go:build windows

package metadata

import "os"

func resolveHome() string {
	home := Get().Paths.Home.Resolve()
	return os.ExpandEnv(home)
}
