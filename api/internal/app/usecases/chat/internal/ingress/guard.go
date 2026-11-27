package ingress

import (
	"cmp"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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
	if channel != engineagents.ChannelHooks {
		return nil, false
	}
	body, ok := agent.ToolDenial(reason)
	return []byte(body), ok
}

func guardReason(dir, now string) string {
	return fmt.Sprintf("Crowbar moved this conversation out of %s; it is no longer yours to write to. "+
		"Work in %s instead.", dir, now)
}
