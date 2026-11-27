package binpath

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

const gitName = "git"

const xcrunShimPath = "/usr/bin/git"

const xcodeSelectLink = "/var/db/xcode_select_link"

var (
	gitPath      atomic.Pointer[string]
	gitResolveMu sync.Mutex
)

func Git() string {
	if p := gitPath.Load(); p != nil {
		return *p
	}
	gitResolveMu.Lock()
	defer gitResolveMu.Unlock()
	if p := gitPath.Load(); p != nil {
		return *p
	}
	resolved := resolveGit(Resolve(gitName), developerDirs())
	gitPath.Store(&resolved)
	return resolved
}

func InvalidateGit() {
	gitPath.Store(nil)
}

func RecoverGit() bool {
	stale := gitPath.Load()
	if !vanished(stale) {
		return false
	}
	gitResolveMu.Lock()
	defer gitResolveMu.Unlock()
	if current := gitPath.Load(); current != stale {
		return *current != *stale
	}
	resolved := resolveGit(Resolve(gitName), developerDirs())
	gitPath.Store(&resolved)
	return resolved != *stale
}

func vanished(
	p *string,
) bool {
	return p != nil && filepath.IsAbs(*p) && !isExecutableFile(*p)
}

func resolveGit(
	found string,
	dirs []string,
) string {
	if runtime.GOOS != "darwin" || found != xcrunShimPath {
		return found
	}
	if direct := developerGit(dirs); direct != "" {
		return direct
	}
	return found
}

func developerGit(
	dirs []string,
) string {
	for _, dir := range dirs {
		candidate := filepath.Join(dir, "usr", "bin", gitName)
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func developerDirs() []string {
	dirs := make([]string, 0, 4)
	if dir := os.Getenv("DEVELOPER_DIR"); dir != "" {
		dirs = append(dirs, dir)
	}
	if dir, err := os.Readlink(xcodeSelectLink); err == nil {
		dirs = append(dirs, dir)
	}
	return append(dirs,
		"/Library/Developer/CommandLineTools",
		"/Applications/Xcode.app/Contents/Developer",
	)
}
