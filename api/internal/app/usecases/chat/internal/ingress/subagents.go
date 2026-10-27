package ingress

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// errInternalSubagent is a stop of a subagent nothing started and that names
// no type: the CLI's own housekeeping agent, no subagent of the chat's.
var errInternalSubagent = fmt.Errorf("ingress: a harness-internal subagent: %w", seam.ErrHookNotRecorded)

// spawnRows are what a phase of a tool call of kind subagent says of the
// subagent it spawns: its start under the call, its bind to the scope the
// call's end names, or its failure before it ever ran.
func spawnRows(st ledger.State, call ledger.ToolCall, turnID string) ([]ledger.Row, error) {
	if call.Kind != ledger.ToolKindSubagent {
		return nil, nil
	}
	a, seen := st.SpawnedBy(call.ToolID)
	switch {
	case call.Phase == ledger.ToolPre && !seen:
		row, err := ledger.NewRow(ledger.KindSubagent, turnID, "", ledger.SubagentBody{
			Phase: ledger.SubagentStart, SpawnToolID: call.ToolID, Title: call.Title,
		})
		return []ledger.Row{row}, err
	case call.SpawnsScope != "" && (a.Scope != call.SpawnsScope || !seen):
		row, err := ledger.NewRow(ledger.KindSubagent, turnID, call.SpawnsScope, ledger.SubagentBody{
			Phase: ledger.SubagentBound, SpawnToolID: call.ToolID, Title: call.Title,
		})
		return []ledger.Row{row}, err
	case call.Phase == ledger.ToolFail && seen && a.Status == ledger.SubagentStarting:
		row, err := ledger.NewRow(ledger.KindSubagent, a.TurnID, "", ledger.SubagentBody{
			Phase: ledger.SubagentStop, SpawnToolID: call.ToolID, Status: string(ledger.SubagentFailed),
		})
		return []ledger.Row{row}, err
	}
	return nil, nil
}

// mapSubagent records a subagent's own start or stop, once. A start no call
// is known to have spawned runs at once; a stop of one never started is the
// CLI's own unless it names a type.
func mapSubagent(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	if ev.Subagent == nil || ev.Subagent.ID == "" {
		return nil, nil
	}
	sub := ev.Subagent
	known, seen := st.SubagentOf(sub.ID)
	if ev.Kind == engineagents.HookSubagentPre {
		if seen {
			return nil, nil
		}
		turnID, _ := turnFor(st, src, "", ev.TurnID)
		return started(turnID, sub)
	}
	if seen && !known.Status.Live() {
		return nil, nil
	}
	if !seen && sub.AgentType == "" {
		return nil, errInternalSubagent
	}
	var rows []ledger.Row
	turnID := known.TurnID
	if !seen {
		turnID, _ = turnFor(st, src, "", ev.TurnID)
		start, err := started(turnID, sub)
		if err != nil {
			return nil, err
		}
		rows = start
	}
	row, err := ledger.NewRow(ledger.KindSubagent, turnID, sub.ID, ended(ledger.SubagentStop, sub))
	return append(rows, row), err
}

func started(turnID string, sub *engineagents.SubagentEvent) ([]ledger.Row, error) {
	row, err := ledger.NewRow(ledger.KindSubagent, turnID, sub.ID, ledger.SubagentBody{
		Phase: ledger.SubagentStart, Type: sub.AgentType, Title: sub.Title,
	})
	return []ledger.Row{row}, err
}

func ended(phase string, sub *engineagents.SubagentEvent) ledger.SubagentBody {
	body := ledger.SubagentBody{Phase: phase, Type: sub.AgentType, Status: sub.Status, Result: sub.Result}
	if u := sub.Usage; u != nil {
		body.Usage = &ledger.SubagentUsage{Tokens: u.Tokens, ToolUses: u.ToolUses, DurationMs: u.DurationMs}
	}
	return body
}

// mapHandBack is a subagent handing its result back: the result, then the
// provider's turn that answers it, opened as a prompt's would be but with no
// one's words.
func mapHandBack(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	rows, err := resultRows(st, ev.Subagent, src)
	if err != nil {
		return nil, err
	}
	src.Prompt = Prompt{}
	opened, err := openProviderTurn(st, ev.TurnID, src)
	return append(rows, opened...), err
}

// resultRows record sub's handed-back result once; one never started is
// started first, so nothing it said is lost.
func resultRows(st ledger.State, sub *engineagents.SubagentEvent, src Source) ([]ledger.Row, error) {
	if sub == nil || sub.ID == "" {
		return nil, nil
	}
	known, seen := st.SubagentOf(sub.ID)
	if known.Result != "" {
		return nil, nil
	}
	var rows []ledger.Row
	turnID := known.TurnID
	if !seen {
		turnID, _ = turnFor(st, src, "", "")
		start, err := started(turnID, sub)
		if err != nil {
			return nil, err
		}
		rows = start
	}
	row, err := ledger.NewRow(ledger.KindSubagent, turnID, sub.ID, ended(ledger.SubagentResult, sub))
	return append(rows, row), err
}

// mapPlanStep records one step of a plan changed alone, in its scope.
func mapPlanStep(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, error) {
	step := ev.Step
	if step == nil || step.StepID == "" {
		return nil, nil
	}
	turnID, _ := turnFor(st, src, scope, ev.TurnID)
	row, err := ledger.NewRow(ledger.KindPlan, turnID, scope, ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{{
		ID: step.StepID, Text: step.Text, Detail: step.Detail, Status: step.Status,
	}}})
	return []ledger.Row{row}, err
}

// scopeOf is the scope of the call a tool event reports: what it spawns or
// addresses is recorded on its row, by its kind.
func scopeOf(call *ledger.ToolCall, tool *engineagents.ToolEvent) {
	if tool.Kind == ledger.ToolKindSubagent {
		call.SpawnsScope = tool.SpawnsScope
		return
	}
	call.AddressesScope = tool.AddressesScope
}

// boundScope is the scope rows bind a subagent to, if any.
func boundScope(rows []ledger.Row) string {
	for _, r := range rows {
		if r.Kind != ledger.KindSubagent || r.Scope == "" {
			continue
		}
		if b, err := ledger.DecodeBody[ledger.SubagentBody](r); err == nil &&
			(b.Phase == ledger.SubagentBound || b.Phase == ledger.SubagentStart) {
			return r.Scope
		}
	}
	return ""
}
