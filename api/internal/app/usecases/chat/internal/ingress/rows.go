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
	// Command reports whether a prompt's text is one the CLI runs itself.
	Command func(text string) bool
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
	// Unbound is an ask bound to no call: its tool's open calls were each
	// given another input.
	Unbound string
	// Bound is the subagent the rows bound to its scope: what is held for it is now its.
	Bound string
}

// Map decides the rows for one parsed event against the chat's state. It does
// no I/O: payload blobs are written before the Do that calls it, and their
// refs are passed in. Every fact is keyed so a CLI's retry records it once.
func Map(st ledger.State, ev engineagents.CanonicalEvent, src Source, refs BlobRefs) ([]ledger.Row, Effects, error) {
	if src.Scope != "" {
		return mapNested(st, ev, src, refs)
	}
	if m, ok := mappers[ev.Kind]; ok {
		return m(st, ev, src, refs)
	}
	return nil, Effects{}, nil
}

// records is whether Map decides rows for an event of kind.
func records(kind string) bool {
	_, ok := mappers[kind]
	return ok
}

type mapFunc func(ledger.State, engineagents.CanonicalEvent, Source, BlobRefs) ([]ledger.Row, Effects, error)

// mappers is the one table of the events Map decides rows for; the ingest
// gate reads it too, so the two cannot drift apart.
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
	engineagents.HookSubagentPre:     rowsOnly(mapSubagent),
	engineagents.HookSubagentPost:    rowsOnly(mapSubagent),
	engineagents.HookPermission:      scoped(mapAsk),
	engineagents.HookElicitation:     scoped(mapAsk),
	engineagents.HookPlanUpdate:      scopedRows(mapPlan),
	engineagents.HookPlanStep:        scopedRows(mapPlanStep),
	engineagents.HookNotification:    mapNotification,
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

// scoped and scopedRows map an event in the scope of the subagent it names.
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

// mapPromptEvent is a prompt's rows; the provider's own prompt titles the chat.
func mapPromptEvent(st ledger.State, ev engineagents.CanonicalEvent, src Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
	rows, err := mapPrompt(st, ev, src)
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

func mapNotification(st ledger.State, ev engineagents.CanonicalEvent, _ Source, _ BlobRefs) ([]ledger.Row, Effects, error) {
	row, err := ledger.NewRow(ledger.KindMarker, runningTurn(st), "",
		ledger.Marker{Kind: ledger.MarkerNotification, Detail: ev.Message})
	return []ledger.Row{row}, Effects{}, err
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
	if ev.Subagent != nil {
		return nil, nil
	}
	return openProviderTurn(st, ev.TurnID, src)
}

// openProviderTurn is the turn the provider started as providerTurnID, unless
// none may start on src's runner or it was already recorded.
func openProviderTurn(st ledger.State, providerTurnID string, src Source) ([]ledger.Row, error) {
	if !src.Held || !startsOn(st, src.RunnerID) {
		return nil, nil
	}
	if _, seen := st.ProviderTurn(providerTurnID); seen {
		return nil, nil
	}
	rows, err := ledger.EndTurn(st, ledger.TurnStatusDone, ledger.ReasonSuperseded)
	if err != nil {
		return nil, err
	}
	started := ledger.TurnStarted{
		ProviderTurnID: providerTurnID, RunnerID: src.RunnerID, Provider: src.Provider,
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
	end := ledger.TurnEnded{
		Status: string(stopStatus(ev.TurnStatus)), Effort: ev.Effort,
		Diff: lastText(src.Live.Diff, ""),
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
	fx := Effects{Ended: turnID}
	if !closes {
		return rows, fx, nil
	}
	steered, reply, err := steerRows(st, src)
	fx.Reply = reply
	return append(rows, steered...), fx, err
}

// closingRows are what a turn's end records: answers streamed with no row of their
// own, the hook's reply and its thought, and every ask nothing will answer now. One
// message is one whole row; the reply is its last message's.
func closingRows(st ledger.State, turnID, reply string, src Source, outlived bool) ([]ledger.Row, error) {
	said := unsaid(st, turnID, src.Live.Answers)
	last, hasLast := st.LastMessage(turnID)
	whole := hasLast && st.HasChunk("", last, ledger.IndexComplete)
	if n := len(said); n > 0 {
		last, hasLast, whole = said[n-1].ID, true, false
		said[n-1].Text = fuller(reply, said[n-1].Text)
	}
	if !hasLast {
		last = ledger.ReplyID(turnID)
	}
	if reply != "" && !whole && len(said) == 0 {
		said = append(said, liveText{ID: last, Text: reply})
	}
	var rows []ledger.Row
	for _, a := range said {
		row, err := ledger.NewRow(ledger.KindAssistantText, turnID, "",
			ledger.AssistantText{MessageID: a.ID, Index: ledger.IndexComplete, Text: a.Text})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if thought := lastText(src.Live.Reasoning, turnID); thought != "" {
		row, err := ledger.NewRow(ledger.KindReasoning, turnID, "", ledger.Reasoning{MessageID: last, Text: thought})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	abandoned, err := abandonAsks(st, turnID, outlived)
	return append(rows, abandoned...), err
}

// fuller is the longer of a turn end's reply and the stream it names: the
// reply may be another message's text, the stream may have lost frames.
func fuller(reply, streamed string) string {
	if len(reply) > len(streamed) {
		return reply
	}
	return streamed
}

// unsaid are turnID's main-scope answers that streamed with no whole row.
func unsaid(st ledger.State, turnID string, answers []liveText) []liveText {
	var out []liveText
	for _, a := range answers {
		if a.TurnID == turnID && a.Scope == "" && !st.HasChunk("", a.ID, ledger.IndexComplete) {
			out = append(out, a)
		}
	}
	return out
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

// abandonAsks closes the open asks of turnID that its end leaves nothing to
// answer: an end a subagent runs past keeps the held ones (ledger.Ask.OutlivesEnd).
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
	rows, err := closingRows(st, failed.TurnID, reply, src, false)
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
		return rows, Effects{}, nil
	}
	settled, err := ledger.SettleUntaken(st, func(p ledger.Prompt) bool {
		return p.RunnerID == src.RunnerID && p.Delivery != ledger.DeliverySteer
	}, src.Command)
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
	rows, err := closingRows(st, failed.TurnID, "", Source{Live: live}, false)
	if err != nil {
		return nil, err
	}
	settled, err := settleSteer(st, runnerID, ledger.ReasonTurnFailed)
	return append(append(rows, failed), settled...), err
}
