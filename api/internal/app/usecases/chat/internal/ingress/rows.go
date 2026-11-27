package ingress

import (
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type Source struct {
	RunnerID, Provider  string
	SessionID, Persists string
	Channel             engineagents.Channel
	Scope               string
	Prompt              Prompt
	Held                bool
	Steer               func(ledger.Prompt) ([]byte, bool)
	Command             func(text string) bool
	Answerable          bool
	CompactsByPrompt    bool
	Manual              bool
	Live                Unrecorded
	EndsAtAction        bool
}

type Prompt struct {
	Text, Origin string
}

type Unrecorded struct {
	Answers, Reasoning, Diff []liveText
}

type BlobRefs struct{ Input, Result string }

type Effects struct {
	Reply   []byte
	HoldAsk string
	Live    *LiveDelta
	Ended   string
	Title   string
	Unbound string
	Bound   string
}

func Map(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) ([]ledger.Row, Effects, error) {
	if src.Scope != "" {
		return mapNested(st, ev, src, refs)
	}
	m, ok := mappers[ev.Kind]
	if !ok {
		return nil, Effects{}, nil
	}
	rows, fx, err := m(st, ev, src, refs)
	if err != nil || !endsText(ev.Kind) {
		return rows, fx, err
	}
	said, err := textBefore(st, ev, src)
	return append(said, rows...), fx, err
}

func records(kind string) bool {
	_, ok := mappers[kind]
	return ok
}

type mapFunc func(ledger.State, engineagents.CanonicalEvent, Source, BlobRefs) ([]ledger.Row, Effects, error)

var mappers = map[string]mapFunc{
	engineagents.HookUserPrompt:      mapPromptEvent,
	engineagents.HookTurnStop:        withoutRefs(mapStop),
	engineagents.HookTurnFailed:      withoutRefs(mapFailure),
	engineagents.HookIdle:            mapIdleEvent,
	engineagents.HookMessageDelta:    withoutRefs(mapText),
	engineagents.HookMessageComplete: withoutRefs(mapText),
	engineagents.HookReasoningDelta:  withoutRefs(mapText),
	engineagents.HookToolOutputDelta: withoutRefs(mapText),
	engineagents.HookDiffUpdate:      withoutRefs(mapText),
	engineagents.HookCompactPre:      rowsOnly(mapCompaction),
	engineagents.HookCompactPost:     rowsOnly(mapCompaction),
	engineagents.HookSubagentResult:  rowsOnly(mapHandBack),
	engineagents.HookToolPre:         mapToolCall,
	engineagents.HookToolPost:        mapToolCall,
	engineagents.HookToolFail:        mapToolCall,
	engineagents.HookTerminalInput:   mapTerminalInput,
	engineagents.HookSubagentPre:     rowsOnly(mapSubagent),
	engineagents.HookSubagentPost:    rowsOnly(mapSubagent),
	engineagents.HookPermission:      scoped(mapAsk),
	engineagents.HookElicitation:     scoped(mapAsk),
	engineagents.HookPlanUpdate:      scopedRows(mapPlan),
	engineagents.HookPlanStep:        scopedRows(mapPlanStep),
	engineagents.HookNotification:    mapSaid(ledger.MarkerNotification),
	engineagents.HookWarning:         mapSaid(ledger.MarkerWarning),
}

func withoutRefs(f func(ledger.State, engineagents.CanonicalEvent, Source) ([]ledger.Row, Effects, error)) mapFunc {
	return func(st ledger.State, ev engineagents.CanonicalEvent, src Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
		return f(st, ev, src)
	}
}

func rowsOnly(f func(ledger.State, engineagents.CanonicalEvent, Source) ([]ledger.Row, error)) mapFunc {
	return func(st ledger.State, ev engineagents.CanonicalEvent, src Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
		rows, err := f(st, ev, src)
		return rows, Effects{}, err
	}
}

func scoped(f func(ledger.State, engineagents.CanonicalEvent, Source, string) ([]ledger.Row, Effects, error)) mapFunc {
	return func(st ledger.State, ev engineagents.CanonicalEvent, src Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
		return f(st, ev, src, subagentScope(ev))
	}
}

func scopedRows(f func(ledger.State, engineagents.CanonicalEvent, Source, string) ([]ledger.Row, error)) mapFunc {
	return func(st ledger.State, ev engineagents.CanonicalEvent, src Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
		rows, err := f(st, ev, src, subagentScope(ev))
		return rows, Effects{}, err
	}
}

func mapPromptEvent(st ledger.State, ev engineagents.CanonicalEvent, src Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
	rows, err := mapPrompt(st, ev, src)
	if err == nil && ev.Subagent == nil {
		var ended []ledger.Row
		ended, err = promptShellRows(st, ev.Shells)
		rows = append(rows, ended...)
	}
	fx := Effects{}
	if len(rows) > 0 && src.Prompt.Origin == ledger.OriginProvider {
		fx.Title = src.Prompt.Text
	}
	return rows, fx, err
}

func mapIdleEvent(st ledger.State, _ engineagents.CanonicalEvent, src Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
	return mapIdle(st, src)
}

func mapToolCall(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) ([]ledger.Row, Effects, error) {
	rows, err := mapTool(st, ev, src, subagentScope(ev), refs)
	return rows, Effects{}, err
}

func mapSaid(kind string) mapFunc {
	return func(st ledger.State, ev engineagents.CanonicalEvent, _ Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
		row, err := ledger.NewRow(ledger.KindMarker, runningTurn(st), "", ledger.Marker{Kind: kind, Detail: ev.Message})
		return []ledger.Row{row}, Effects{}, err
	}
}

func subagentScope(ev engineagents.CanonicalEvent) string {
	if ev.Subagent == nil {
		return ""
	}
	return ev.Subagent.ID
}

func turnFor(st ledger.State, src Source, scope, providerTurnID string) (string, bool) {
	if scope != "" {
		if id, ok := st.ScopeTurn(scope); ok {
			return id, true
		}
	}
	if id, ok := st.ProviderTurn(providerTurnID); ok {
		return id, true
	}
	if id, ok := st.RunnerTurn(src.RunnerID); ok {
		return id, true
	}
	last, ok := st.LastTurn()
	return last.ID, ok
}

func runningTurn(st ledger.State) string {
	t, _ := st.RunningTurn()
	return t.ID
}

func startsOn(st ledger.State, runnerID string) bool {
	live, ok := st.LiveRunner()
	return ok && live.RunnerID == runnerID
}

func mapPrompt(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	if ev.Subagent != nil {
		return nil, nil
	}
	return openProviderTurn(st, ev.TurnID, src)
}

func stopStatus(providerStatus string) ledger.TurnStatus {
	if providerStatus == "" || providerStatus == "completed" {
		return ledger.TurnStatusDone
	}
	return ledger.TurnStatusInterrupted
}

func compactionEnd(st ledger.State, providerTurnID string) ([]ledger.Row, bool, error) {
	_, armed := st.Compaction()
	if armed == "" || armed != providerTurnID {
		return nil, false, nil
	}
	row, err := ledger.NewRow(ledger.KindMarker, "", "",
		ledger.Marker{Kind: ledger.MarkerCompactionEnded, ProviderTurnID: providerTurnID})
	return []ledger.Row{row}, true, err
}

func compactionEndedWith(st ledger.State, turnID string) ([]ledger.Row, error) {
	if !st.CompactionIn(turnID) {
		return nil, nil
	}
	row, err := ledger.NewRow(ledger.KindMarker, turnID, "", ledger.Marker{Kind: ledger.MarkerCompactionEnded})
	return []ledger.Row{row}, err
}

func mapStop(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, Effects, error) {
	if rows, ok, err := compactionEnd(st, ev.TurnID); ok || err != nil {
		return rows, Effects{}, err
	}
	end := ledger.TurnEnded{
		Status: string(stopStatus(ev.TurnStatus)), Effort: ev.Effort,
		Diff: lastText(src.Live.Diff, ""), ProviderTurnID: ev.TurnID,
	}
	turnID, ended, closes, err := ledger.EndTurnOf(st, src.RunnerID, ev.TurnID, end)
	if err != nil {
		return nil, Effects{}, err
	}
	rows, err := closingRows(st, turnID, ev.Message, src, st.SubagentRunning())
	if err != nil {
		return nil, Effects{}, err
	}
	rows = append(rows, ended...)
	compacted, err := compactionEndedWith(st, turnID)
	if err != nil {
		return nil, Effects{}, err
	}
	rows = append(rows, compacted...)
	fx := Effects{Ended: turnID}
	if !closes {
		return rows, fx, nil
	}
	steered, reply, err := steerRows(st, src)
	fx.Reply = reply
	return append(rows, steered...), fx, err
}

func lastText(texts []liveText, turnID string) string {
	out := ""
	for _, t := range texts {
		if turnID == "" || t.TurnID == turnID {
			out = t.Text
		}
	}
	return out
}

func abandonAsks(st ledger.State, turnID string, outlived bool) ([]ledger.Row, error) {
	var rows []ledger.Row
	for _, a := range st.PendingAsks() {
		if a.TurnID != turnID || a.OutlivesEnd(outlived) {
			continue
		}
		resolved, err := ledger.ResolveAsk(st, a.AskID, ledger.ResolutionAbandoned, nil)
		if err != nil {
			return nil, err
		}
		rows = append(rows, resolved...)
	}
	return rows, nil
}

func mapIdle(st ledger.State, src Source) ([]ledger.Row, Effects, error) {
	turnID, rows, closes, err := ledger.EndTurnOf(st, src.RunnerID, "", ledger.TurnEnded{
		Status: string(ledger.TurnStatusDone), Reason: ledger.ReasonProviderIdle,
	})
	if err != nil {
		return nil, Effects{}, err
	}
	if closes {
		closing, err := closingRows(st, turnID, "", src, false)
		if err != nil {
			return nil, Effects{}, err
		}
		settled, err := settleSteer(st, src.RunnerID, ledger.ReasonDeliveryFailed)
		return append(append(closing, rows...), settled...), Effects{Ended: turnID}, err
	}
	if _, running := st.RunningTurn(); running || src.Channel == engineagents.ChannelAPI {
		return nil, Effects{}, nil
	}
	settled, err := ledger.SettleUntaken(st, func(p ledger.Prompt) bool {
		return p.RunnerID == src.RunnerID && p.Delivery != ledger.DeliverySteer
	}, src.Command)
	return settled, Effects{}, err
}

func Salvage(st ledger.State, answers []liveText) ([]ledger.Row, error) {
	var rows []ledger.Row
	for _, a := range answers {
		if a.TurnID == "" || st.HasChunk(a.Scope, a.ID, ledger.IndexComplete) {
			continue
		}
		row, err := ledger.NewRow(ledger.KindAssistantText, a.TurnID, a.Scope,
			ledger.AssistantText{MessageID: a.ID, Index: ledger.IndexComplete, Text: a.Text})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}
