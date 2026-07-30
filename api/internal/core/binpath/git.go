package binpath

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

const gitName = "git"

// xcrunShimPath is the only git on a stock macOS PATH, and it is not git: since
// Xcode 4.3 every developer tool under /usr/bin is a stub that re-resolves the
// active developer directory and then exec's the real binary out of it. That
// re-resolution runs on EVERY invocation: `git rev-parse HEAD` measures 9.8ms
// through the shim against 3.5ms run directly, so the stub costs nearly twice
// what git does. The daemon spawns git for the diff subsystem, the sidebar,
// merge-eligibility and workspace creation, so it is paid tens of times per
// user interaction.
const xcrunShimPath = "/usr/bin/git"

// xcodeSelectLink is the symlink `xcode-select -s` maintains and `xcode-select
// -p` prints. Reading it is how the active developer directory is discovered
// without spawning xcrun — a resolution that itself forked a process per git
// call would cost more than the shim it replaces.
const xcodeSelectLink = "/var/db/xcode_select_link"

var (
	gitPath      atomic.Pointer[string]
	gitResolveMu sync.Mutex
)

// Git returns the path to exec for a git subprocess, resolved once per process
// and cached. Every subsequent call is a lock-free pointer load; nothing is
// spawned, at first use or after, and nothing is persisted — the answer is a
// property of this machine at this moment, not of the user's configuration.
//
// On macOS the resolution deliberately looks PAST the /usr/bin shim to the
// developer-directory binary the shim itself would have exec'd, so the git that
// runs is byte-identical to the git that ran before, reached without the stub.
// Everywhere else it is the plain PATH lookup, returned as an absolute path.
//
// When nothing resolves it returns "git" unchanged, so the caller's exec fails
// exactly as it does today. Resolution never turns a working git call into a
// failing one.
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

// InvalidateGit drops the cached resolution so the next Git call derives it
// again.
//
// The daemon itself never needs this: it repairs launchd's minimal PATH with
// the login-shell PATH before anything execs an external tool (cmd/crowbar
// runServe), and nothing moves afterwards. It exists because the cache makes
// that ordering a real constraint — anything that deliberately changes the
// environment a resolution was derived from, PATH above all, keeps receiving
// the answer from before the change until it says so.
func InvalidateGit() {
	gitPath.Store(nil)
}

// RecoverGit re-resolves git when the cached path has stopped naming an
// executable file, and reports whether the caller should retry the invocation
// that just failed to start.
//
// The case it exists for is an Xcode or Command Line Tools update landing while
// the daemon is running: the developer directory is replaced wholesale, and a
// path resolved before the update can name a file that no longer exists.
// Without recovery a single background update would fail every git call for the
// remaining life of the process.
//
// It is deliberately driven by a stat of the cached path rather than by the
// exec error, because a missing working directory produces the same
// "fork/exec: no such file or directory" as a missing binary and must not be
// mistaken for one. A caller whose repo directory was deleted sees its real
// error, unretried.
func RecoverGit() bool {
	p := gitPath.Load()
	if p == nil || !filepath.IsAbs(*p) || isExecutableFile(*p) {
		return false
	}
	gitResolveMu.Lock()
	defer gitResolveMu.Unlock()
	resolved := resolveGit(Resolve(gitName), developerDirs())
	gitPath.Store(&resolved)
	return resolved != *p
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

// developerDirs lists candidate developer directories in the order xcrun itself
// consults them: the DEVELOPER_DIR override first, then the xcode-select link,
// then the two stock install locations as a last resort for a machine whose
// link is missing. Only the first entry that actually contains an executable
// git is used, so a stale entry costs one stat.
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
