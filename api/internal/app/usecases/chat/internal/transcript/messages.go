package transcript

import (
	"cmp"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// noteSpeaker is who a note Crowbar adds to the conversation is shown from.
const noteSpeaker = "crowbar"

// MessageID is the id a message streamed as messageID is listed under: the
// id its live frames name, prefixed, so a client joins the two.
func MessageID(messageID string) string { return "msg-" + messageID }

// turnFacts is one turn as its rows left it.
type turnFacts struct {
	start            ledger.TurnStarted
	started          bool
	startSeq, endSeq int64
	startAt, endAt   time.Time
	ended, failed    bool
	async            bool   // its first end declared background work
	status           string // the component status its first end gave it
	effort, diff     string
	plan             []ledger.PlanItem
	planAt           time.Time
}

type userFact struct {
	seq    int64
	at     time.Time
	turnID string
	body   ledger.UserMessage
}

type noticeFact struct {
	seq    int64
	at     time.Time
	turnID string
	text   string
}

func (f *facts) applyUser(r ledger.Row) {
	if body, err := ledger.DecodeBody[ledger.UserMessage](r); err == nil && r.Scope == "" {
		f.users = append(f.users, userFact{seq: r.Seq, at: r.At, turnID: r.TurnID, body: body})
	}
}

func (f *facts) applyStart(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.TurnStarted](r)
	if err != nil || r.Scope != "" {
		return
	}
	if t := f.turn(r.TurnID); !t.started {
		t.start, t.started, t.startSeq, t.startAt = body, true, r.Seq, r.At
	}
}

func (f *facts) applyFailed(t *turnFacts, r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.TurnFailed](r)
	if err != nil {
		return
	}
	if !t.failed {
		f.notes = append(f.notes, noticeFact{seq: r.Seq, at: r.At, turnID: r.TurnID, text: cmp.Or(body.Detail, body.Reason)})
	}
	t.failed = true
	f.end(t, r, "failed", ledger.TurnEnded{})
}

// applyEnd records a turn's first end; a failure is also a notice of its own.
func (f *facts) applyEnd(r ledger.Row) {
	if r.Scope != "" {
		return
	}
	t := f.turn(r.TurnID)
	if r.Kind == ledger.KindTurnFailed {
		f.applyFailed(t, r)
		return
	}
	body, err := ledger.DecodeBody[ledger.TurnEnded](r)
	if err != nil {
		return
	}
	status := "completed"
	if body.Status == string(ledger.TurnStatusInterrupted) {
		status = "interrupted"
	}
	f.end(t, r, status, body)
}

func (f *facts) end(t *turnFacts, r ledger.Row, status string, body ledger.TurnEnded) {
	t.effort, t.diff = cmp.Or(body.Effort, t.effort), cmp.Or(body.Diff, t.diff)
	if !t.started && t.start.RunnerID == "" && body.RunnerID != "" {
		// A turn whose start never came is its ending runner's.
		t.start.RunnerID, t.start.Provider = body.RunnerID, f.runners[body.RunnerID]
	}
	if !t.ended {
		t.ended, t.endSeq, t.endAt, t.status = true, r.Seq, r.At, status
		t.async = body.AsyncWork != nil && *body.AsyncWork > 0
	}
}

func (f *facts) applyPlan(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.Plan](r)
	if err != nil || r.Scope != "" || r.TurnID == "" {
		return
	}
	t := f.turn(r.TurnID)
	if body.Op == ledger.PlanUpsert {
		t.plan = upsert(t.plan, body.Items)
	} else {
		t.plan = slices.Clone(body.Items)
	}
	t.planAt = r.At
}

func upsert(plan, items []ledger.PlanItem) []ledger.PlanItem {
	out := slices.Clone(plan)
	for _, it := range items {
		at := slices.IndexFunc(out, func(p ledger.PlanItem) bool { return it.ID != "" && p.ID == it.ID })
		if at < 0 {
			out = append(out, it)
			continue
		}
		out[at] = it
	}
	return out
}

// listed is one message in both its read shapes.
type listed struct {
	msg  domain.LedgerMessage
	turn domain.ActivityTurn
}

// messages lists every message a client is shown: a person's once its turn
// started, the agent's once settled, each turn's failure as a notice.
func (f *facts) messages() ([]domain.LedgerMessage, []domain.ActivityTurn) {
	var items []listed
	for _, u := range f.users {
		if it, ok := f.userMessage(u); ok {
			items = append(items, it)
		}
	}
	last := map[string]int{}
	for _, m := range f.text.Messages() {
		if m.Key.Scope != "" || !m.Settled || m.Text == "" {
			continue
		}
		if i, ok := last[m.Key.TurnID]; !ok || items[i].turn.DisplayOrder < m.FirstSeq {
			last[m.Key.TurnID] = len(items)
		}
		items = append(items, f.agentMessage(m))
	}
	for turnID, i := range last {
		f.attach(&items[i].turn, turnID)
	}
	for _, n := range f.notes {
		items = append(items, f.notice(n))
	}
	slices.SortStableFunc(items, func(a, b listed) int {
		return cmp.Or(cmp.Compare(a.msg.Sequence, b.msg.Sequence), cmp.Compare(a.msg.DisplayOrder, b.msg.DisplayOrder))
	})
	messages := make([]domain.LedgerMessage, 0, len(items))
	turns := make([]domain.ActivityTurn, 0, len(items))
	for _, it := range items {
		messages, turns = append(messages, it.msg), append(turns, it.turn)
	}
	return messages, turns
}

func (f *facts) userMessage(u userFact) (listed, bool) {
	t := f.turns[u.turnID]
	if t == nil {
		t = &turnFacts{}
	}
	id, seq, at, role := u.turnID, u.seq, u.at, domain.TurnRoleUser
	switch u.body.Origin {
	case ledger.OriginCrowbar:
		// The client queued it under its request; it is said once taken.
		if !t.started {
			return listed{}, false
		}
		id, seq, at = u.body.RequestID, t.startSeq, t.startAt
	case ledger.OriginHarness:
		role = domain.TurnRoleHarness
	}
	provider := t.start.Provider
	if u.body.Origin == ledger.OriginNote {
		provider = noteSpeaker
	}
	lt := domain.LedgerTurn{
		ID: id, Role: role, Provider: provider, RunnerID: t.start.RunnerID,
		SessionID: t.start.SessionID, Text: u.body.Text, At: at,
	}
	return f.item(lt, seq, seq, "completed", at), true
}

func (f *facts) agentMessage(m ledger.MessageText) listed {
	t := f.turns[m.Key.TurnID]
	if t == nil {
		t = &turnFacts{}
	}
	lt := domain.LedgerTurn{
		ID: MessageID(m.Key.MessageID), Role: domain.TurnRoleAssistant, Provider: t.start.Provider,
		RunnerID: t.start.RunnerID, SessionID: t.start.SessionID, Text: m.Text, Effort: t.effort, At: m.At,
	}
	status := "completed"
	if t.ended {
		status = t.status
	}
	return f.item(lt, m.FirstSeq, m.SettleSeq, status, m.SettledAt)
}

func (f *facts) notice(n noticeFact) listed {
	t := f.turns[n.turnID]
	if t == nil {
		t = &turnFacts{}
	}
	lt := domain.LedgerTurn{
		ID: "notice-" + n.turnID, Role: domain.TurnRoleNotice, Provider: t.start.Provider,
		RunnerID: t.start.RunnerID, SessionID: t.start.SessionID, Text: n.text, At: n.at,
	}
	return f.item(lt, n.seq, n.seq, "failed", n.at)
}

func (f *facts) item(lt domain.LedgerTurn, order, seq int64, status string, endedAt time.Time) listed {
	return listed{
		msg: domain.LedgerMessage{Sequence: int(seq), DisplayOrder: order, LedgerTurn: lt},
		turn: domain.ActivityTurn{
			ID: lt.ID, ChatID: f.chatID, Seq: seq, DisplayOrder: order, Role: lt.Role,
			ProviderID: lt.Provider, RunnerID: lt.RunnerID, SessionID: lt.SessionID,
			Text: lt.Text, Effort: lt.Effort, Status: status, StartedAt: lt.At, EndedAt: &endedAt,
		},
	}
}

// attach gives a turn's last message what the turn as a whole produced: its
// latest plan and its diff.
func (f *facts) attach(turn *domain.ActivityTurn, turnID string) {
	t := f.turns[turnID]
	if t == nil {
		return
	}
	turn.Diff = t.diff
	if len(t.plan) == 0 {
		return
	}
	turn.Plan = make([]domain.ActivityPlanStep, 0, len(t.plan))
	for _, it := range t.plan {
		turn.Plan = append(turn.Plan, domain.ActivityPlanStep{Text: it.Text, Status: it.Status})
	}
	at := t.planAt
	turn.PlanUpdatedAt = &at
}

// page is the window of messages, by Sequence, after or before a cursor: the
// limit messages following after, else the limit newest before before (0 is
// the end). Messages sharing a Sequence are never split across pages.
func page(messages []domain.LedgerMessage, after, before, limit int) domain.LedgerPage {
	var window []domain.LedgerMessage
	var hasMore bool
	if after > 0 {
		window, hasMore = pageAfter(messages, after, limit)
	} else {
		window, hasMore = pageBefore(messages, before, limit)
	}
	out := domain.LedgerPage{HasMore: hasMore, Items: slices.Clone(window)}
	if out.Items == nil {
		out.Items = []domain.LedgerMessage{}
	}
	if n := len(out.Items); n > 0 {
		out.OldestCursor, out.Cursor = out.Items[0].Sequence, out.Items[n-1].Sequence
	}
	return out
}

func pageAfter(messages []domain.LedgerMessage, after, limit int) ([]domain.LedgerMessage, bool) {
	start, _ := slices.BinarySearchFunc(messages, after+1, bySequence)
	end := min(start+limit, len(messages))
	for end < len(messages) && end > start && messages[end].Sequence == messages[end-1].Sequence {
		end++
	}
	return messages[start:end], end < len(messages)
}

func pageBefore(messages []domain.LedgerMessage, before, limit int) ([]domain.LedgerMessage, bool) {
	end := len(messages)
	if before > 0 {
		end, _ = slices.BinarySearchFunc(messages, before, bySequence)
	}
	start := max(end-limit, 0)
	for start > 0 && start < end && messages[start].Sequence == messages[start-1].Sequence {
		start--
	}
	return messages[start:end], start > 0
}

func bySequence(m domain.LedgerMessage, seq int) int { return cmp.Compare(m.Sequence, seq) }
