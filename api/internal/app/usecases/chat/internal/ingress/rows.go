package ingress

import (
	"cmp"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Source is what the dispatcher read before the rows were decided: who sent
// the event, and everything about it the ledger cannot answer.
type Source struct {
	RunnerID, Provider string
	// SessionID is the chat's bound session with Provider; Persists how a turn
	// on it makes it resumable.
	SessionID, Persists string
	Channel             engineagents.Channel
	// Scope is the nested conversation an event of another session belongs to.
	Scope string
	// Prompt is a user_prompt's words as the dispatcher read them.
	Prompt Prompt
	// Held: the runner is held against its end, so a turn may open on it.
	Held bool
	// Steer is the hook reply handing a steered prompt to the CLI; false when
	// the event's channel cannot carry one.
	Steer func(ledger.Prompt) ([]byte, bool)
	// Answerable: Crowbar can hold an ask's reply.
	Answerable bool
	// CompactsByPrompt: a compaction is asked for with a prompt no turn
	// acknowledges, so its start spends the prompt.
	CompactsByPrompt bool
	// Manual: a person asked for the chat's next compaction.
	Manual bool
	// Live is the runner's streamed text no row holds, for a turn's end.
	Live Unrecorded
}

// Prompt is a user_prompt's words. Origin is OriginProvider for words a person
// typed, OriginHarness for the provider's own, and "" for Crowbar's injected
// context alone: a turn starts, and nobody said anything.
type Prompt struct {
	Text, Origin string
}

// Unrecorded is a runner's streamed text no row holds, by kind.
type Unrecorded struct {
	Answers, Reasoning, Diff []liveText
}

// BlobRefs are the refs of a tool call's payloads, written before its rows.
type BlobRefs struct{ Input, Result string }

// Effects are what the dispatcher does once the rows are committed.
type Effects struct {
	Reply   []byte     // the hook's stdout: a steered prompt
	HoldAsk string     // the ask whose reply Crowbar holds
	Live    *LiveDelta // the text the event streamed
	// Ended is the turn whose end spent its streamed text.
	Ended string
	// Title is the words of a prompt that opened a turn: they title the chat.
	Title string
}

// Map decides the rows for one parsed event against the chat's state. It does
// no I/O: payload blobs are written before the Do that calls it, and their
// refs are passed in. Every fact is keyed so a CLI's retry records it once.
func Map(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) ([]ledger.Row, Effects, error) {
	if src.Scope != "" {
		return mapNested(st, ev, src, refs)
	}
	switch ev.Kind {
	case engineagents.HookUserPrompt:
		rows, err := mapPrompt(st, ev, src)
		fx := Effects{}
		if len(rows) > 0 && src.Prompt.Origin == ledger.OriginProvider {
			fx.Title = src.Prompt.Text
		}
		return rows, fx, err
	case engineagents.HookTurnStop:
		return mapStop(st, ev, src)
	case engineagents.HookTurnFailed:
		return mapFailure(st, ev, src)
	case engineagents.HookIdle:
		return mapIdle(st, src)
	case engineagents.HookMessageDelta, engineagents.HookMessageComplete,
		engineagents.HookReasoningDelta, engineagents.HookToolOutputDelta, engineagents.HookDiffUpdate:
		return mapText(st, ev, src)
	case engineagents.HookCompactPre, engineagents.HookCompactPost:
		rows, err := mapCompaction(st, ev, src)
		return rows, Effects{}, err
	}
	return mapActivity(st, ev, src, refs)
}

// mapActivity decides the rows of what the agent did inside a turn.
func mapActivity(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) ([]ledger.Row, Effects, error) {
	scope := subagentScope(ev)
	switch ev.Kind {
	case engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail:
		rows, err := mapTool(st, ev, src, scope, refs)
		return rows, Effects{}, err
	case engineagents.HookSubagentPre, engineagents.HookSubagentPost:
		rows, err := mapSubagent(st, ev, src)
		return rows, Effects{}, err
	case engineagents.HookPermission, engineagents.HookElicitation:
		return mapAsk(st, ev, src, scope)
	case engineagents.HookPlanUpdate:
		rows, err := mapPlan(st, ev, src, scope)
		return rows, Effects{}, err
	case engineagents.HookNotification:
		row, err := ledger.NewRow(ledger.KindMarker, runningTurn(st), "",
			ledger.Marker{Kind: ledger.MarkerNotification, Detail: ev.Message})
		return []ledger.Row{row}, Effects{}, err
	}
	return nil, Effects{}, nil
}

func subagentScope(ev engineagents.CanonicalEvent) string {
	if ev.Subagent == nil {
		return ""
	}
	return ev.Subagent.ID
}

// turnFor is the turn a fact of src's runner belongs to: the one its provider
// turn id names, else the newest its runner ran, else the chat's newest.
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

// startsOn reports whether a turn may start on runnerID's CLI: only the
// chat's live runner's.
func startsOn(st ledger.State, runnerID string) bool {
	live, ok := st.LiveRunner()
	return ok && live.RunnerID == runnerID
}

func mapPrompt(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	// A prompt inside a subagent is its parent talking to it; a turn its
	// provider already started here is a retry.
	if ev.Subagent != nil || !src.Held || !startsOn(st, src.RunnerID) {
		return nil, nil
	}
	if _, seen := st.ProviderTurn(ev.TurnID); seen {
		return nil, nil
	}
	rows, err := ledger.EndTurn(st, ledger.TurnStatusDone, ledger.ReasonSuperseded)
	if err != nil {
		return nil, err
	}
	started := ledger.TurnStarted{
		ProviderTurnID: ev.TurnID, RunnerID: src.RunnerID, Provider: src.Provider,
		SessionID: src.SessionID, Persists: src.Persists,
	}
	opened, err := openRows(st, started, src)
	return append(rows, opened...), err
}

// openRows start the turn the prompt opens: on the oldest pending Crowbar
// prompt its runner carries when the words are that prompt's.
func openRows(st ledger.State, started ledger.TurnStarted, src Source) ([]ledger.Row, error) {
	switch src.Prompt.Origin {
	case "":
		row, err := ledger.NewRow(ledger.KindTurnStarted, uuid.NewString(), "", started)
		return []ledger.Row{row}, err
	case ledger.OriginProvider:
		for _, p := range st.PendingPrompts() {
			if p.Text != src.Prompt.Text || (p.RunnerID != "" && p.RunnerID != src.RunnerID) {
				continue
			}
			started.RequestID = p.RequestID
			row, err := ledger.NewRow(ledger.KindTurnStarted, p.TurnID, "", started)
			return []ledger.Row{row}, err
		}
	}
	turnID := uuid.NewString()
	msg, err := ledger.NewRow(ledger.KindUserMessage, turnID, "",
		ledger.UserMessage{Text: src.Prompt.Text, Origin: src.Prompt.Origin})
	if err != nil {
		return nil, err
	}
	row, err := ledger.NewRow(ledger.KindTurnStarted, turnID, "", started)
	return []ledger.Row{msg, row}, err
}

// stopStatus is how a turn_stop's provider status ended its turn: anything but
// a completed turn did not finish its work.
func stopStatus(providerStatus string) ledger.TurnStatus {
	if providerStatus == "" || providerStatus == "completed" {
		return ledger.TurnStatusDone
	}
	return ledger.TurnStatusInterrupted
}

// compactionEnd is the marker a compaction's own turn ends as: that turn's
// end is never a reply or a failure.
func compactionEnd(st ledger.State, providerTurnID string) ([]ledger.Row, bool, error) {
	_, armed := st.Compaction()
	if armed == "" || armed != providerTurnID {
		return nil, false, nil
	}
	row, err := ledger.NewRow(ledger.KindMarker, "", "",
		ledger.Marker{Kind: ledger.MarkerCompactionEnded, ProviderTurnID: providerTurnID})
	return []ledger.Row{row}, true, err
}

func mapStop(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, Effects, error) {
	if rows, ok, err := compactionEnd(st, ev.TurnID); ok || err != nil {
		return rows, Effects{}, err
	}
	ended, closes, err := ledger.EndTurnOf(st, src.RunnerID, ev.TurnID, ledger.TurnEnded{
		Status: string(stopStatus(ev.TurnStatus)), AsyncWork: &ev.AsyncWork, Effort: ev.Effort,
		Diff: lastText(src.Live.Diff, ""),
	})
	if err != nil {
		return nil, Effects{}, err
	}
	turnID := ended.TurnID
	rows, err := closingRows(st, turnID, ev.Message, src)
	if err != nil {
		return nil, Effects{}, err
	}
	rows = append(rows, ended)
	fx := Effects{Ended: turnID}
	if !closes {
		return rows, fx, nil
	}
	steered, reply, err := steerRows(st, src)
	fx.Reply = reply
	return append(rows, steered...), fx, err
}

// closingRows are what a turn's end records of what it said: the answers
// streamed with no row of their own, the reply its hook carried, the thought
// behind it, and every ask nothing will answer now.
func closingRows(st ledger.State, turnID, reply string, src Source) ([]ledger.Row, error) {
	var rows []ledger.Row
	last, hasLast := st.LastMessage(turnID)
	for _, a := range src.Live.Answers {
		if a.TurnID != turnID || a.Scope != "" || st.HasChunk("", a.ID, ledger.IndexComplete) {
			continue
		}
		row, err := ledger.NewRow(ledger.KindAssistantText, turnID, "",
			ledger.AssistantText{MessageID: a.ID, Index: ledger.IndexComplete, Text: a.Text})
		if err != nil {
			return nil, err
		}
		rows, last, hasLast = append(rows, row), a.ID, true
	}
	messageID := last
	if !hasLast {
		messageID = ledger.ReplyID(turnID)
	}
	if reply != "" {
		row, err := ledger.NewRow(ledger.KindAssistantText, turnID, "",
			ledger.AssistantText{MessageID: messageID, Index: ledger.IndexComplete, Text: reply})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if thought := lastText(src.Live.Reasoning, turnID); thought != "" {
		row, err := ledger.NewRow(ledger.KindReasoning, turnID, "", ledger.Reasoning{MessageID: messageID, Text: thought})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	abandoned, err := abandonAsks(st, turnID)
	return append(rows, abandoned...), err
}

// lastText is the newest of texts, of turnID's turn when one is named.
func lastText(texts []liveText, turnID string) string {
	out := ""
	for _, t := range texts {
		if turnID == "" || t.TurnID == turnID {
			out = t.Text
		}
	}
	return out
}

// abandonAsks close turnID's open asks: its end leaves nothing to answer them.
func abandonAsks(st ledger.State, turnID string) ([]ledger.Row, error) {
	var rows []ledger.Row
	for _, a := range st.PendingAsks() {
		if a.TurnID != turnID {
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

func mapFailure(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, Effects, error) {
	if rows, ok, err := compactionEnd(st, ev.TurnID); ok || err != nil {
		return rows, Effects{}, err
	}
	fail := ledger.TurnFailed{Reason: ledger.ReasonTurnFailed, Detail: ev.Message}
	reply := ""
	if ev.Failure != nil {
		fail.Reason = cmp.Or(ev.Failure.Reason, fail.Reason)
		fail.Detail = cmp.Or(ev.Failure.Detail, ev.Message)
		if ev.Failure.Detail != "" {
			reply = ev.Message // the provider's explanation is the detail; this is its reply
		}
	}
	failed, closes, err := ledger.FailTurnOf(st, src.RunnerID, ev.TurnID, fail)
	if err != nil {
		return nil, Effects{}, err
	}
	rows, err := closingRows(st, failed.TurnID, reply, src)
	if err != nil {
		return nil, Effects{}, err
	}
	rows = append(rows, failed)
	if closes {
		settled, err := settleSteer(st, src.RunnerID, ledger.ReasonTurnFailed)
		if err != nil {
			return nil, Effects{}, err
		}
		rows = append(rows, settled...)
	}
	return rows, Effects{Ended: failed.TurnID}, nil
}

// mapIdle is the provider saying it is idle: its running turn ends as any end
// does, any other end is late. With no turn, a hooks CLI's report means the
// prompts it carried were never taken; an api connection's, nothing.
func mapIdle(st ledger.State, src Source) ([]ledger.Row, Effects, error) {
	ended, closes, err := ledger.EndTurnOf(st, src.RunnerID, "", ledger.TurnEnded{
		Status: string(ledger.TurnStatusDone), Reason: ledger.ReasonProviderIdle,
	})
	if err != nil {
		return nil, Effects{}, err
	}
	if closes {
		rows, err := closingRows(st, ended.TurnID, "", src)
		if err != nil {
			return nil, Effects{}, err
		}
		settled, err := settleSteer(st, src.RunnerID, ledger.ReasonDeliveryFailed)
		return append(append(rows, ended), settled...), Effects{Ended: ended.TurnID}, err
	}
	rows := []ledger.Row{ended}
	if _, running := st.RunningTurn(); running || src.Channel == engineagents.ChannelAPI {
		return rows, Effects{}, nil
	}
	settled, err := ledger.Settle(st, func(p ledger.Prompt) bool {
		return p.RunnerID == src.RunnerID && p.Delivery != ledger.DeliverySteer
	}, false, ledger.ReasonNotAcknowledged)
	return append(rows, settled...), Effects{}, err
}

// Salvage is what a runner's exit records of the answers it streamed with no
// row of their own, so its exit keeps what it said.
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

// noticeRows fail the turn runnerID runs for what its screen says, or with
// none running, record the notice as a failure of its own and settle the
// prompts it carries unconsumed.
func noticeRows(st ledger.State, runnerID string, notice engineagents.TerminalNotice, live Unrecorded) ([]ledger.Row, error) {
	fail := ledger.TurnFailed{Reason: notice.Kind, Detail: notice.Text}
	if _, running := st.RunningTurn(); !running {
		failed, _, err := ledger.FailTurnOf(st, runnerID, uuid.NewString(), fail)
		if err != nil {
			return nil, err
		}
		settled, err := ledger.SettleCarried(st, runnerID, false, notice.Kind)
		return append([]ledger.Row{failed}, settled...), err
	}
	failed, closes, err := ledger.FailTurnOf(st, runnerID, "", fail)
	if err != nil || !closes {
		return nil, err
	}
	rows, err := closingRows(st, failed.TurnID, "", Source{Live: live})
	if err != nil {
		return nil, err
	}
	settled, err := settleSteer(st, runnerID, ledger.ReasonTurnFailed)
	return append(append(rows, failed), settled...), err
}
