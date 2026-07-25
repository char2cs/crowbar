package agent

import (
	"os"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
)

// Connected reports whether a provider's CLI is installed: its spawn.cmd resolves
// to an executable file on PATH. binpath.Resolve also probes well-known bin dirs
// (the daemon's launchd-minimal PATH misses ~/.local/bin, where claude/codex
// install), so a real install is never a false negative. Install-only — there is
// deliberately no auth check: the claude/codex CLIs expose no reliable
// machine-readable one, so a logged-out-but-installed CLI reads connected and
// fails at spawn (the existing 424 toast explains it).
func Connected(
	cmd string,
) bool {
	info, err := os.Stat(binpath.Resolve(cmd))
	return err == nil && !info.IsDir()
}
