package ingress

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// leftFolderWrite is the folder a tool call would write into that its chat moved
// out of, "" when none. An edit names its file; a command is matched on the paths
// it spells out, which is best-effort: a shell can reach a folder without naming it.
func leftFolderWrite(st ledger.State, ev engineagents.CanonicalEvent) string {
	left := st.LeftDirs()
	if len(left) == 0 || ev.Kind != engineagents.HookToolPre || ev.Tool == nil {
		return ""
	}
	switch ev.Tool.Kind {
	case "edit":
		return within(left, ev.Tool.Target, ev.Tool.Cwd)
	case "execute":
		for _, word := range strings.Fields(ev.Tool.Target) {
			if dir := within(left, strings.Trim(word, `"'`), ""); dir != "" {
				return dir
			}
		}
	}
	return ""
}

func within(dirs []string, path, cwd string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		if cwd == "" {
			return ""
		}
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	for _, d := range dirs {
		if path == d || strings.HasPrefix(path, d+string(filepath.Separator)) {
			return d
		}
	}
	return ""
}

// guardReason is what the agent is told when a write into dir is refused.
func guardReason(dir, now string) string {
	return fmt.Sprintf("Crowbar moved this conversation out of %s; it is no longer yours to write to. "+
		"Work in %s instead.", dir, now)
}
