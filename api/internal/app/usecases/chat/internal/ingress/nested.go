package ingress

import (
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// mapNested decides the rows of an event from a nested conversation the
// chat's agent opened: its tool calls, its asks and its end, recorded in its
// own scope. Its streamed text is not part of the chat's record.
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

// nestedEnd is a nested conversation's turn ending: what it said, and the
// stop of the scope it ran as, in the turn that opened it or else its runner's.
func nestedEnd(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, error) {
	turnID, ok := turnFor(st, src, scope, "")
	if !ok {
		return nil, errNoTurn
	}
	var rows []ledger.Row
	if ev.Message != "" {
		row, err := ledger.NewRow(ledger.KindAssistantText, turnID, scope, ledger.AssistantText{
			MessageID: ledger.ReplyID(scope), Index: ledger.IndexComplete, Text: ev.Message,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if st.HasScopePhase(scope, ledger.SubagentStop) {
		return rows, nil
	}
	row, err := ledger.NewRow(ledger.KindSubagent, turnID, scope, ledger.Subagent{Phase: ledger.SubagentStop})
	return append(rows, row), err
}
