package ingress

import (
	"cmp"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// errNoTurn is a message that names no turn the chat ever had: nothing can
// hold it, so it is dead-lettered whole.
var errNoTurn = fmt.Errorf("ingress: a message of no turn: %w", seam.ErrHookNotRecorded)

// mapText decides what streamed text records: a numbered chunk or a whole
// message is a row; anything else is only shown while it happens.
func mapText(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, Effects, error) {
	scope := subagentScope(ev)
	if ev.Kind == engineagents.HookMessageComplete {
		rows, err := mapComplete(st, ev, src, scope)
		return rows, Effects{}, err
	}
	d := ev.Delta
	if d == nil {
		return nil, Effects{}, nil
	}
	if ev.Kind == engineagents.HookMessageDelta && d.Sequenced {
		return mapChunk(st, ev, src, scope)
	}
	turnID, _ := turnFor(st, src, scope, d.TurnID)
	delta := LiveDelta{TurnID: turnID, Scope: scope, ID: d.MessageID, Index: d.Index, Text: d.Text, ShownAs: d.MessageID}
	switch ev.Kind {
	case engineagents.HookReasoningDelta:
		delta.Kind = KindReasoning
	case engineagents.HookToolOutputDelta:
		delta.Kind = KindToolOutput
	case engineagents.HookDiffUpdate:
		delta.Kind, delta.Replace = KindDiff, true
	}
	if delta.Kind != KindAnswer {
		// One undifferentiated stream per turn names no block: the turn groups it.
		delta.ID = cmp.Or(d.MessageID, d.TurnID)
		delta.ShownAs = delta.ID
	}
	if delta.ID == "" && delta.Kind == KindAnswer || d.Text == "" && !delta.Replace {
		return nil, Effects{}, nil
	}
	return nil, Effects{Live: &delta}, nil
}

// mapChunk records chunk Index of a message, once. A message that began after
// its turn closed on a reply it reads as names that reply, which it joins.
func mapChunk(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, Effects, error) {
	d := ev.Delta
	if d.MessageID == "" || st.HasChunk(scope, d.MessageID, d.Index) {
		return nil, Effects{}, nil
	}
	turnID, known := st.MessageTurn(scope, d.MessageID)
	shown := st.ShownAs(scope, d.MessageID)
	reply := ""
	if !known {
		var ok bool
		if turnID, ok = turnFor(st, src, scope, d.TurnID); !ok {
			return nil, Effects{}, errNoTurn
		}
		reply = joinedReply(st, scope, turnID, d)
		shown = cmp.Or(reply, shown)
	}
	row, err := ledger.NewRow(ledger.KindAssistantText, turnID, scope, ledger.AssistantText{
		MessageID: d.MessageID, Index: d.Index, Final: d.Final, Text: d.Text, Reply: reply,
	})
	if err != nil {
		return nil, Effects{}, err
	}
	live := &LiveDelta{
		TurnID: turnID, Scope: scope, ID: d.MessageID, Kind: KindAnswer,
		Index: d.Index, Numbered: true, Text: d.Text, ShownAs: shown,
	}
	if shown != d.MessageID {
		_, live.Floor, _ = st.Reply(turnID)
	}
	return []ledger.Row{row}, Effects{Live: live}, nil
}

// joinedReply is the reply of turnID a message it began after closing joins.
func joinedReply(st ledger.State, scope, turnID string, d *engineagents.MessageDelta) string {
	if id, text, has := st.Reply(turnID); has && scope == "" && (d.Index != 0 || ledger.Related(d.Text, text)) {
		return id
	}
	return ""
}

// mapComplete records a message the provider reported whole, once.
func mapComplete(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, error) {
	c := ev.Complete
	if c == nil || c.MessageID == "" || st.HasChunk(scope, c.MessageID, ledger.IndexComplete) {
		return nil, nil
	}
	turnID, ok := st.MessageTurn(scope, c.MessageID)
	if !ok {
		if turnID, ok = turnFor(st, src, scope, c.TurnID); !ok {
			return nil, errNoTurn
		}
	}
	row, err := ledger.NewRow(ledger.KindAssistantText, turnID, scope, ledger.AssistantText{
		MessageID: c.MessageID, Index: ledger.IndexComplete, Text: c.Text, Phase: c.Phase,
	})
	return []ledger.Row{row}, err
}

func toolPhase(kind string) string {
	switch kind {
	case engineagents.HookToolPost:
		return ledger.ToolPost
	case engineagents.HookToolFail:
		return ledger.ToolFail
	}
	return ledger.ToolPre
}

// mapTool records one phase of a tool call in its scope, once. A call's end
// closes the asks it raised: the CLI went on.
func mapTool(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string, refs BlobRefs) ([]ledger.Row, error) {
	tool := ev.Tool
	if tool == nil {
		return nil, nil
	}
	phase := toolPhase(ev.Kind)
	id := tool.ID
	if id == "" && phase != ledger.ToolPre {
		id, _ = st.OpenTool(scope, tool.Name, "")
	}
	if id == "" {
		id = "tool-" + uuid.NewString()
	}
	if st.HasToolPhase(scope, id, phase) {
		return nil, nil
	}
	turnID, _ := turnFor(st, src, scope, ev.TurnID)
	body := ledger.ToolCall{
		ToolID: id, Phase: phase, Name: tool.Name, Kind: tool.Kind, Title: tool.Title, Target: tool.Target,
		Locations: toolLocations(tool), InputRef: refs.Input, ResultRef: refs.Result,
		Error: caption(tool.Error), DurationMS: tool.DurationMS, SpawnsScope: tool.NestedSessionID,
		ExitCode: tool.ExitCode, Interrupted: tool.Interrupted,
	}
	if !st.HasToolInput(scope, id) {
		// A call is keyed once: its first input is the one an ask names it by.
		body.InputKey = ledger.InputKey(tool.Input)
	}
	if tool.Kind == ledger.ToolKindMCP {
		// Its kind makes a call an MCP one: other kinds name a tool too.
		body.MCPServer, body.MCPTool = tool.MCPServer, tool.MCPTool
	}
	if phase == ledger.ToolPre {
		body.Diff = toolStartDiff(tool)
	} else {
		body.Diff, body.Status = toolResultDiff(tool), toolStatus(ev)
	}
	row, err := ledger.NewRow(ledger.KindToolCall, turnID, scope, body)
	if err != nil || phase == ledger.ToolPre {
		return []ledger.Row{row}, err
	}
	rows := []ledger.Row{row}
	for _, askID := range st.AsksOfTool(scope, id) {
		resolved, err := ledger.ResolveAsk(st, askID, ledger.ResolutionProceeded, nil)
		if err != nil {
			return nil, err
		}
		rows = append(rows, resolved...)
	}
	return rows, nil
}

func toolLocations(tool *engineagents.ToolEvent) []ledger.ToolLocation {
	if len(tool.Locations) == 0 {
		return nil
	}
	out := make([]ledger.ToolLocation, 0, len(tool.Locations))
	for _, l := range tool.Locations {
		out = append(out, ledger.ToolLocation{Path: l.Path, Line: l.Line})
	}
	return out
}

func toolStatus(ev engineagents.CanonicalEvent) string {
	if ev.Kind == engineagents.HookToolFail {
		if ev.Tool.Status == domain.ToolStatusDeclined {
			return domain.ToolStatusDeclined
		}
		return domain.ToolStatusError
	}
	return cmp.Or(ev.Tool.Status, domain.ToolStatusOK)
}

// mapSubagent records a subagent's start or stop in its own scope, once.
func mapSubagent(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	if ev.Subagent == nil || ev.Subagent.ID == "" {
		return nil, nil
	}
	scope := ev.Subagent.ID
	phase := ledger.SubagentStart
	if ev.Kind == engineagents.HookSubagentPost {
		phase = ledger.SubagentStop
	}
	if st.HasScopePhase(scope, phase) {
		return nil, nil
	}
	turnID, _ := turnFor(st, src, scope, ev.TurnID)
	row, err := ledger.NewRow(ledger.KindSubagent, turnID, scope,
		ledger.Subagent{Phase: phase, Type: ev.Subagent.AgentType})
	return []ledger.Row{row}, err
}

// mapPlan records the agent's plan as restated, in its scope.
func mapPlan(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, error) {
	items := make([]ledger.PlanItem, 0, len(ev.Plan))
	for _, step := range ev.Plan {
		items = append(items, ledger.PlanItem{Text: step.Text, Status: step.Status})
	}
	turnID, _ := turnFor(st, src, scope, ev.TurnID)
	row, err := ledger.NewRow(ledger.KindPlan, turnID, scope, ledger.Plan{Op: ledger.PlanReplace, Items: items})
	return []ledger.Row{row}, err
}

// mapCompaction opens and closes a compaction's marker, naming the turn it runs
// as, whose end is then the compaction's. One asked for with a prompt spends
// that prompt: no turn acknowledges it.
func mapCompaction(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	open, _ := st.Compaction()
	detail := ""
	if ev.Interrupt != nil {
		detail = ev.Interrupt.Detail
	}
	var rows []ledger.Row
	switch {
	case ev.Kind == engineagents.HookCompactPre && !open:
		marker := ledger.Marker{Kind: ledger.MarkerCompactionStarted, Detail: detail}
		if src.Manual && detail == "" {
			marker.Detail = "manual"
		}
		running := runningTurn(st)
		if running == "" {
			marker.ProviderTurnID = ev.TurnID
		}
		row, err := ledger.NewRow(ledger.KindMarker, running, "", marker)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	case ev.Kind == engineagents.HookCompactPost && open:
		row, err := ledger.NewRow(ledger.KindMarker, runningTurn(st), "",
			ledger.Marker{Kind: ledger.MarkerCompactionEnded, Detail: detail})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if !src.CompactsByPrompt {
		return rows, nil
	}
	settled, err := ledger.SettleCarried(st, src.RunnerID, true, ledger.ReasonProviderBuiltin)
	return append(rows, settled...), err
}
