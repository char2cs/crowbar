package ingress

import (
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func mapNested(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) ([]ledger.Row, Effects, error) {
	scope := src.Scope
	switch ev.Kind {
	case engineagents.HookPermission, engineagents.HookElicitation:
		return mapAsk(st, ev, src, scope)
	case engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail:
		rows, err := mapTool(st, ev, src, scope, refs)
		return rows, Effects{}, err
	case engineagents.HookTurnStop, engineagents.HookTurnFailed:
		rows, err := nestedEnd(st, ev, src, scope)
		return rows, Effects{}, err
	}
	return nil, Effects{}, nil
}

func nestedEnd(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, error) {
	turnID, ok := turnFor(st, src, scope, "")
	if !ok {
		return nil, errNoTurn
	}
	a, seen := st.SubagentOf(scope)
	var rows []ledger.Row
	if ev.Message != "" && (!seen || !a.Status.Live()) {
		row, err := ledger.NewRow(ledger.KindAssistantText, turnID, scope, ledger.AssistantText{
			MessageID: ledger.ReplyID(scope), Index: ledger.IndexComplete, Text: ev.Message,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if seen && !a.Status.Live() {
		return rows, nil
	}
	stop := ledger.SubagentBody{Phase: ledger.SubagentStop, Result: ev.Message}
	if ev.Kind == engineagents.HookTurnFailed {
		stop.Status = string(ledger.SubagentFailed)
	}
	row, err := ledger.NewRow(ledger.KindSubagent, turnID, scope, stop)
	return append(rows, row), err
}
