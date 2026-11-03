package binpath

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

const gitName = "git"

// xcrunShimPath is the only git on a stock macOS PATH, and it is a stub that re-resolves the developer
// directory on every call, costing about twice what git does; the daemon spawns git constantly.
const xcrunShimPath = "/usr/bin/git"

// xcodeSelectLink is the symlink `xcode-select -s` maintains; reading it finds the developer directory
// without spawning xcrun.
const xcodeSelectLink = "/var/db/xcode_select_link"

var (
	gitPath      atomic.Pointer[string]
	gitResolveMu sync.Mutex
)

// Git returns the git path to exec, resolved once per process and cached (a lock-free load after). On
// macOS it looks past the /usr/bin shim to the binary the shim would exec. When nothing resolves it
// returns "git", so resolution never turns a working call into a failing one.
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

// InvalidateGit drops the cached resolution so the next Git call derives it again. The daemon repairs
// PATH before any exec and never needs it; anything that later changes PATH must call it.
func InvalidateGit() {
	gitPath.Store(nil)
}

// RecoverGit re-resolves git when the cached path no longer names an executable (e.g. an Xcode update
// replaced the developer directory) and reports whether to retry. It stats the path rather than reading
// the exec error, which a missing working directory produces identically.
func RecoverGit() bool {
	stale := gitPath.Load()
	if !vanished(stale) {
		return false
	}
	gitResolveMu.Lock()
	defer gitResolveMu.Unlock()
	// Every in-flight call fails together after an update; an unchanged pointer means no one has
	// re-resolved yet, so only one caller redoes the PATH lookup.
	if current := gitPath.Load(); current != stale {
		return *current != *stale
	}
	resolved := resolveGit(Resolve(gitName), developerDirs())
	gitPath.Store(&resolved)
	return resolved != *stale
}

// vanished reports whether a cached resolution stopped naming an executable. A bare name never counts:
// it is the unresolvable fallback, and recovering on it would re-resolve on every failure.
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

// developerDirs lists candidate developer directories in xcrun's order: DEVELOPER_DIR, the
// xcode-select link, then the stock locations. The first containing an executable git wins.
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
