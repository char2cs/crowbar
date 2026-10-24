package ingress

import (
	"cmp"
	"encoding/json"
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
		id, _ = st.OpenTool(scope, tool.Name)
	}
	if id == "" {
		id = "tool-" + uuid.NewString()
	}
	if st.HasToolPhase(scope, id, phase) {
		return nil, nil
	}
	turnID, _ := turnFor(st, src, scope, ev.TurnID)
	body := ledger.ToolCall{
		ToolID: id, Phase: phase, Name: tool.Name, Kind: tool.Kind, Target: tool.Target,
		Locations: toolLocations(tool), InputRef: refs.Input, ResultRef: refs.Result,
		Error: tool.Error, DurationMS: tool.DurationMS, SpawnsScope: tool.NestedSessionID,
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

// mapAsk records a question the CLI is blocked on, bound to the open tool of
// its scope that raised it. A retried delivery joins the ask it repeats.
func mapAsk(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, Effects, error) {
	a, err := askOf(ev)
	if err != nil {
		return nil, Effects{}, err
	}
	if a.ToolName != "" {
		a.ToolID, _ = st.OpenTool(scope, a.ToolName)
	}
	if id, ok := st.SameAsk(scope, a); ok {
		return nil, Effects{HoldAsk: heldAgain(st, id, src)}, nil
	}
	live := startsOn(st, src.RunnerID)
	held := src.Answerable && a.Kind != "" && live
	a.AskID = "choice-" + uuid.NewString()
	a.Answerable = held
	turnID, _ := turnFor(st, src, scope, ev.TurnID)
	row, err := ledger.NewRow(ledger.KindAsk, turnID, scope, a)
	if err != nil {
		return nil, Effects{}, err
	}
	if held {
		return []ledger.Row{row}, Effects{HoldAsk: a.AskID}, nil
	}
	if live && waitedOn(st, scope) {
		return []ledger.Row{row}, Effects{}, nil
	}
	// Unheld, a past runner's or one no end closes: nothing waits on it.
	closed, err := ledger.NewRow(ledger.KindAskResolved, turnID, scope,
		ledger.AskResolved{AskID: a.AskID, Resolution: ledger.ResolutionAbandoned})
	if err != nil {
		return nil, Effects{}, err
	}
	return []ledger.Row{row, closed}, Effects{}, nil
}

// waitedOn reports whether an ask in scope is waited on: the chat's agent's
// while a turn runs, a nested conversation's while no turn end outlived it.
func waitedOn(st ledger.State, scope string) bool {
	if scope == "" || !st.HasScopePhase(scope, ledger.SubagentStart) {
		_, running := st.RunningTurn()
		return running
	}
	return st.ScopeWaits(scope)
}

// heldAgain is the ask a repeated delivery's reply is held on: the one it
// repeats, while Crowbar can still hold it.
func heldAgain(st ledger.State, askID string, src Source) string {
	for _, p := range st.PendingAsks() {
		if p.AskID == askID && p.Answerable && src.Answerable {
			return askID
		}
	}
	return ""
}

func askOf(ev engineagents.CanonicalEvent) (ledger.Ask, error) {
	a := ledger.Ask{Event: ev.Kind}
	if ev.Interrupt != nil {
		a.Detail = ev.Interrupt.Detail
	}
	c := ev.Choice
	if c == nil {
		return a, nil
	}
	a.Kind, a.PromptID, a.ToolName = c.Kind, c.PromptID, c.ToolName
	a.Title, a.Question, a.Mode, a.Multi = c.Title, c.Question, c.Mode, c.Multi
	var err error
	if len(c.Options) > 0 {
		if a.Options, err = json.Marshal(choiceOptions(c.Options)); err != nil {
			return a, fmt.Errorf("ingress: ask options: %w", err)
		}
	}
	if len(c.Questions) > 0 {
		if a.Questions, err = json.Marshal(choiceQuestions(c.Questions)); err != nil {
			return a, fmt.Errorf("ingress: ask questions: %w", err)
		}
	}
	if json.Valid(c.Schema) {
		a.Schema = json.RawMessage(c.Schema)
	}
	return a, nil
}

func choiceQuestions(in []engineagents.PromptQuestion) []domain.ActivityChoiceQuestion {
	out := make([]domain.ActivityChoiceQuestion, 0, len(in))
	for _, q := range in {
		out = append(out, domain.ActivityChoiceQuestion{
			ID: q.ID, Title: q.Title, Text: q.Text, Multi: q.Multi, Options: choiceOptions(q.Options),
		})
	}
	return out
}

func choiceOptions(in []engineagents.ChoiceOption) []domain.ActivityChoiceOption {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.ActivityChoiceOption, 0, len(in))
	for _, o := range in {
		out = append(out, domain.ActivityChoiceOption{ID: o.ID, Kind: o.Kind, Label: o.Label, Description: o.Description})
	}
	return out
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

// mapCompaction opens and closes a compaction's marker. One that runs as a
// turn of its own names that turn, whose end is then the compaction's; one
// inside a running turn names none. A compaction asked for with a prompt
// spends that prompt: no turn acknowledges it.
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
