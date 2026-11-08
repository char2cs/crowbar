package ingress

import (
	"cmp"
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
	if len(left) == 0 {
		return ""
	}
	kind, target, cwd := aim(st, ev)
	switch kind {
	case ledger.ToolKindEdit:
		return within(left, target, cwd)
	case "execute":
		for _, word := range strings.Fields(target) {
			if dir := within(left, strings.Trim(word, `"'`), ""); dir != "" {
				return dir
			}
		}
	}
	return ""
}

// aim is what the call ev is about to run does and to what: its own, or for an ask
// about a call, that call's as its first phase recorded it.
func aim(st ledger.State, ev engineagents.CanonicalEvent) (kind, target, cwd string) {
	switch {
	case ev.Kind == engineagents.HookToolPre && ev.Tool != nil:
		return ev.Tool.Kind, ev.Tool.Target, ev.Tool.Cwd
	case ev.Kind == engineagents.HookPermission && ev.Choice != nil && ev.Choice.ToolName != "":
		scope := ""
		if st.ScopeOpen(ev.SessionID) {
			scope = ev.SessionID
		}
		id, _, _ := st.OpenTool(scope, ev.Choice.ToolName, ledger.InputKey(ev.Choice.ToolInput))
		kind, target := st.ToolAim(scope, id)
		return kind, cmp.Or(ev.Choice.ToolTarget, target), ""
	}
	return "", "", ""
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

// guardLeftFolder is the reply refusing ev when it writes into a folder the chat left,
// before any person is asked: a call's own hook refuses it, and so does an ask about it.
func guardLeftFolder(
	st ledger.State, agent engineagents.Agent, ev engineagents.CanonicalEvent, raw []byte, channel engineagents.Channel,
) ([]byte, bool) {
	dir := leftFolderWrite(st, ev)
	if dir == "" {
		return nil, false
	}
	reason := guardReason(dir, st.MovedTo())
	if ev.Kind == engineagents.HookPermission {
		reply, err := agent.RenderAnswer(ev.Kind, raw, engineagents.AnswerDecision{
			Key: engineagents.ChoiceOptionDeny, Reason: reason,
		})
		return reply, err == nil
	}
	// A notice that a call started cannot refuse it: only a hook's reply can.
	if channel != engineagents.ChannelHooks {
		return nil, false
	}
	body, ok := agent.ToolDenial(reason)
	return []byte(body), ok
}

// guardReason is what the agent is told when a write into dir is refused.
func guardReason(dir, now string) string {
	return fmt.Sprintf("Crowbar moved this conversation out of %s; it is no longer yours to write to. "+
		"Work in %s instead.", dir, now)
}
