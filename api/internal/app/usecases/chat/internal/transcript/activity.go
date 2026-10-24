package transcript

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// The interruption kind a compaction is shown as.
const kindCompaction = "compaction"

// shownMarkers are the markers a client draws. A session move and a refused
// report are the daemon's own record, not the chat's.
var shownMarkers = map[string]bool{
	ledger.MarkerStopped: true, ledger.MarkerProviderSwitched: true, ledger.MarkerModelChanged: true,
	ledger.MarkerEffortChanged: true, ledger.MarkerNotification: true,
}

// scoped names a tool call: its scope and its own id.
type scoped struct{ scope, id string }

type toolFact struct {
	key    scoped
	turnID string
	seq    int64
	call   domain.ActivityToolCall
	closed bool
}

type scopeFact struct {
	id, turnID, agentType string
	seq                   int64
	startAt, stopAt       time.Time
	// opened: a start or a spawning tool call named it, not only a stop.
	opened, stopped bool
}

type markFact struct {
	seq                  int64
	at                   time.Time
	turnID, kind, detail string
	resolvedAt           *time.Time
}

func (f *facts) applyTool(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.ToolCall](r)
	if err != nil || body.ToolID == "" {
		return
	}
	key := scoped{scope: r.Scope, id: body.ToolID}
	t, ok := f.tools[key]
	if !ok {
		t = &toolFact{key: key, turnID: r.TurnID, seq: r.Seq, call: domain.ActivityToolCall{StartedAt: r.At}}
		f.tools[key] = t
		f.order = append(f.order, t)
	}
	c := &t.call
	c.Name, c.Kind, c.Target = cmp.Or(c.Name, body.Name), cmp.Or(c.Kind, body.Kind), cmp.Or(c.Target, body.Target)
	c.RequestRef, c.ResultRef = cmp.Or(c.RequestRef, body.InputRef), cmp.Or(body.ResultRef, c.ResultRef)
	c.Diff = cmp.Or(body.Diff, c.Diff)
	if len(c.Locations) == 0 {
		for _, l := range body.Locations {
			c.Locations = append(c.Locations, domain.ActivityToolLocation{Path: l.Path, Line: l.Line})
		}
	}
	if body.Phase != ledger.ToolPre && !t.closed {
		at := r.At
		t.closed, c.Status, c.Error, c.DurationMS, c.EndedAt = true, body.Status, body.Error, body.DurationMS, &at
	}
	if body.SpawnsScope != "" {
		f.scope(body.SpawnsScope, r).opened = true
	}
}

func (f *facts) scope(id string, r ledger.Row) *scopeFact {
	s, ok := f.scopes[id]
	if !ok {
		s = &scopeFact{id: id, turnID: r.TurnID, seq: r.Seq, startAt: r.At}
		f.scopes[id] = s
		f.spawns = append(f.spawns, s)
	}
	return s
}

func (f *facts) applySubagent(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.Subagent](r)
	if err != nil || r.Scope == "" {
		return
	}
	s := f.scope(r.Scope, r)
	s.agentType = cmp.Or(s.agentType, body.Type)
	s.opened = s.opened || body.Phase == ledger.SubagentStart
	if body.Phase == ledger.SubagentStop && !s.stopped {
		s.stopped, s.stopAt = true, r.At
	}
}

func (f *facts) applyMarker(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.Marker](r)
	if err != nil {
		return
	}
	switch body.Kind {
	case ledger.MarkerCompactionStarted:
		if f.compacting != nil {
			return
		}
		f.compacting = &markFact{seq: r.Seq, at: r.At, turnID: r.TurnID, kind: kindCompaction, detail: body.Detail}
		f.marks = append(f.marks, f.compacting)
	case ledger.MarkerCompactionEnded:
		if f.compacting != nil {
			at := r.At
			f.compacting.resolvedAt, f.compacting = &at, nil
		}
	default:
		if !shownMarkers[body.Kind] {
			return
		}
		// Crowbar's own doing and what the CLI only reports: over as it is said.
		at := r.At
		f.marks = append(f.marks, &markFact{
			seq: r.Seq, at: r.At, turnID: r.TurnID, kind: body.Kind, detail: body.Detail, resolvedAt: &at,
		})
	}
}

// toolCalls are the chat's tool calls by their first row. The main agent's
// are shown with the message that follows them; a scope's with its subagent.
func (f *facts) toolCalls() []domain.ActivityToolCall {
	following := f.mainMessages()
	out := make([]domain.ActivityToolCall, 0, len(f.order))
	for _, t := range f.order {
		c := t.call
		c.ID, c.ChatID, c.Seq = t.key.id, f.chatID, t.seq
		if t.key.scope != "" {
			c.SubagentID = t.key.scope
		} else {
			c.TurnID = followingMessage(following[t.turnID], t.turnID, t.seq)
		}
		if !t.closed {
			c.Status = domain.ToolStatusRunning
			if at, over := f.toolEnd(t); over {
				c.Status, c.EndedAt = domain.ToolStatusAbandoned, &at
			}
		}
		out = append(out, c)
	}
	return out
}

// mainMessages are each turn's main-scope messages, by their first row.
func (f *facts) mainMessages() map[string][]ledger.MessageText {
	out := map[string][]ledger.MessageText{}
	for _, m := range f.text.Messages() {
		if m.Key.Scope == "" {
			out[m.Key.TurnID] = append(out[m.Key.TurnID], m)
		}
	}
	return out
}

// followingMessage is the id of the first of a turn's messages begun after
// seq, else of the reply its end would record.
func followingMessage(messages []ledger.MessageText, turnID string, seq int64) string {
	at := slices.IndexFunc(messages, func(m ledger.MessageText) bool { return m.FirstSeq > seq })
	if at < 0 {
		return MessageID(ledger.ReplyID(turnID))
	}
	return MessageID(messages[at].Key.MessageID)
}

// toolEnd is when an open tool call is over: when its scope is, the main
// scope's being its turn's.
func (f *facts) toolEnd(t *toolFact) (time.Time, bool) {
	if s, ok := f.scopes[t.key.scope]; ok && t.key.scope != "" {
		return f.scopeEnd(s)
	}
	return f.closedBy(t.turnID, t.seq, false)
}

// scopeEnd is when subagent s is over: at its stop, else at its turn's end
// when it opened before an end that declared no background work, else at its
// runner's exit.
func (f *facts) scopeEnd(s *scopeFact) (time.Time, bool) {
	if s.stopped {
		return s.stopAt, true
	}
	t := f.turns[s.turnID]
	if t != nil && t.ended && t.endSeq > s.seq && !t.async {
		return t.endAt, true
	}
	return f.exitAfter(t, s.seq)
}

// subagents are the chat's opened subagents by their first row, each with
// what it said, and over as scopeEnd says.
func (f *facts) subagents() []domain.ActivitySubagent {
	said := map[string][]domain.ActivitySubagentMessage{}
	for _, m := range f.text.Messages() {
		if m.Key.Scope != "" && m.Settled && m.Text != "" {
			said[m.Key.Scope] = append(said[m.Key.Scope], domain.ActivitySubagentMessage{Text: m.Text, At: m.SettledAt})
		}
	}
	out := make([]domain.ActivitySubagent, 0, len(f.spawns))
	for _, s := range f.spawns {
		if !s.opened {
			continue // the CLI's own housekeeping agents report only their stop
		}
		a := domain.ActivitySubagent{
			ID: s.id, TurnID: s.turnID, ChatID: f.chatID, Seq: s.seq, AgentType: s.agentType,
			StartedAt: s.startAt, Messages: said[s.id],
		}
		if at, over := f.scopeEnd(s); over {
			a.EndedAt = &at
		}
		out = append(out, a)
	}
	return out
}

// interruptions are the chat's markers and asks, by their row.
func (f *facts) interruptions() []domain.ActivityInterruption {
	out := make([]domain.ActivityInterruption, 0, len(f.marks)+len(f.asked))
	for _, m := range f.marks {
		resolved := m.resolvedAt
		if resolved == nil {
			if at, over := f.closedBy(m.turnID, m.seq, true); over {
				resolved = &at
			}
		}
		out = append(out, domain.ActivityInterruption{
			ID: fmt.Sprintf("m-%d", m.seq), TurnID: m.turnID, ChatID: f.chatID, Seq: m.seq, DisplayOrder: m.seq,
			Kind: m.kind, Detail: m.detail, At: m.at, ResolvedAt: resolved,
		})
	}
	for _, a := range f.asked {
		resolvedAt, _, _ := f.askResolution(a)
		out = append(out, domain.ActivityInterruption{
			ID: InterruptionID(a.ask.AskID), TurnID: a.turnID, ChatID: f.chatID, Seq: a.seq, DisplayOrder: a.seq,
			Kind: a.ask.Event, Detail: a.ask.Detail, At: a.at, ResolvedAt: resolvedAt,
		})
	}
	slices.SortFunc(out, func(a, b domain.ActivityInterruption) int { return cmp.Compare(a.Seq, b.Seq) })
	return out
}
