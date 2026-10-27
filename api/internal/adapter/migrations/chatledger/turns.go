package chatledger

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// The legacy transcript's roles and an assistant message's end statuses.
const (
	roleUser      = "user"
	roleAssistant = "assistant"
	roleHarness   = "harness"
	roleNotice    = "notice"

	statusFailed      = "failed"
	statusInterrupted = "interrupted"
	statusAbandoned   = "abandoned"

	kindInferred = "inferred"
	// reasonNotice ends a turn the old daemon closed with a notice row.
	reasonNotice = "notice"
)

// group is one ledger turn: the user or harness row that opened it and the
// assistant rows that answered, until the next opening row.
type group struct {
	id, runnerID, provider string
	head                   *LegacyTurn
	replies                []LegacyTurn
	notice                 *LegacyTurn
	start, end             time.Time
	inferred               *LegacyInterruption
}

// turns restates the legacy transcript, in its display order, as turns.
func (c *conversion) turns(in LegacyChat) {
	rows := slices.Clone(in.Turns)
	slices.SortStableFunc(rows, func(a, b LegacyTurn) int {
		return cmp.Or(cmp.Compare(a.DisplayOrder, b.DisplayOrder), cmp.Compare(a.ItemIndex, b.ItemIndex), cmp.Compare(a.Seq, b.Seq))
	})
	c.byID = map[string]*group{}
	var g *group
	for i := range rows {
		row := &rows[i]
		switch row.Role {
		case roleUser, roleHarness:
			g = c.open(row)
		case roleAssistant:
			if g == nil {
				g = c.open(row)
			}
			g.replies = append(g.replies, *row)
		case roleNotice:
			c.notice(g, row)
		default:
			c.skipf("turn %s: unknown role %q", row.ID, row.Role)
			continue
		}
		if row.ID != "" {
			c.byID[row.ID] = g
		}
	}
	c.inferEnds(in.Interruptions)
	var floor time.Time
	for _, g := range c.groups {
		floor = c.restate(g, in.Prompts, floor)
	}
}

func (c *conversion) open(head *LegacyTurn) *group {
	g := &group{id: turnID(c.chatID, cmp.Or(head.ID, fmt.Sprintf("seq-%d", head.Seq))), head: head, runnerID: head.RunnerID, provider: head.ProviderID}
	c.groups = append(c.groups, g)
	return g
}

// notice ends the turn it is in as failed; outside a turn it is what the CLI
// told its user.
func (c *conversion) notice(g *group, row *LegacyTurn) {
	if g == nil {
		c.emit(ruleMarker, row.StartedAt, row.Seq, ledger.KindMarker, "", "",
			ledger.Marker{Kind: ledger.MarkerNotification, Detail: row.Text})
		return
	}
	g.notice = row
}

// inferEnds gives an inferred interruption to the turn it names when that turn
// recorded no end of its own.
func (c *conversion) inferEnds(interruptions []LegacyInterruption) {
	for i, in := range interruptions {
		if in.Kind != kindInferred {
			continue
		}
		g := c.turnOf(in.TurnID, in.At)
		if g == nil || g.inferred != nil || g.ended() {
			c.skipf("interruption %s: inferred, and its turn has an end", in.ID)
			continue
		}
		g.inferred = &interruptions[i]
	}
}

// ended reports whether the turn's last reply recorded how it ended.
func (g *group) ended() bool {
	if g.notice != nil {
		return true
	}
	if len(g.replies) == 0 {
		return false
	}
	last := g.replies[len(g.replies)-1]
	return last.EndedAt != nil || last.Status == statusFailed || last.Status == statusInterrupted || last.Status == statusAbandoned
}

// restate emits g's rows no earlier than floor, past the turn before it, and
// returns the instant after its end.
func (c *conversion) restate(g *group, prompts []LegacyPrompt, floor time.Time) time.Time {
	g.provider = cmp.Or(g.provider, firstProvider(g.replies))
	g.runnerID = cmp.Or(g.runnerID, firstRunner(g.replies))
	g.start = latest(g.head.StartedAt, floor)
	head := g.head
	if head.Role != roleAssistant {
		c.emit(ruleTurnStart, g.start, head.Seq, ledger.KindUserMessage, g.id, "", userMessage(head, prompts))
	}
	started := ledger.TurnStarted{
		RunnerID: g.runnerID, Provider: g.provider, SessionID: c.sessionOf(g.provider, head.SessionID),
	}
	if m := userMessage(head, prompts); m.Origin == ledger.OriginCrowbar {
		started.RequestID = m.RequestID
	}
	c.emit(ruleTurnStart, g.start, head.Seq, ledger.KindTurnStarted, g.id, "", started)
	g.end = g.start
	for _, r := range g.replies {
		at := latest(r.StartedAt, g.start)
		c.emit(ruleText, at, r.Seq, ledger.KindAssistantText, g.id, "",
			ledger.AssistantText{MessageID: cmp.Or(r.ID, fmt.Sprintf("seq-%d", r.Seq)), Index: ledger.IndexComplete, Final: true, Text: r.Text})
		g.end = latest(g.end, at, endOf(r))
	}
	if g.notice != nil {
		g.end = latest(g.end, g.notice.StartedAt)
	}
	if g.inferred != nil {
		g.end = latest(g.end, g.inferred.At)
	}
	c.restatePlan(g)
	kind, body := g.ending()
	c.emit(ruleTurnEnd, g.end, 0, kind, g.id, "", body)
	return g.end.Add(time.Microsecond)
}

// userMessage is the row a turn's opening row is: a crowbar prompt when the
// journal holds its request, the harness's or the provider's otherwise.
func userMessage(head *LegacyTurn, prompts []LegacyPrompt) ledger.UserMessage {
	m := ledger.UserMessage{Text: head.Text, Origin: ledger.OriginProvider}
	switch {
	case head.Role == roleHarness:
		m.Origin = ledger.OriginHarness
	case slices.ContainsFunc(prompts, func(p LegacyPrompt) bool { return p.RequestID == head.ID }):
		m.Origin, m.RequestID = ledger.OriginCrowbar, head.ID
	}
	return m
}

// restatePlan records the turn's last plan before its end.
func (c *conversion) restatePlan(g *group) {
	var raw string
	for _, r := range g.replies {
		raw = cmp.Or(r.Plan, raw)
	}
	if raw == "" || raw == "null" {
		return
	}
	var steps []struct {
		Text   string `json:"text"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &steps); err != nil {
		c.skipf("turn %s: plan unreadable: %v", g.head.ID, err)
		return
	}
	items := make([]ledger.PlanItem, 0, len(steps))
	for _, s := range steps {
		items = append(items, ledger.PlanItem{Text: s.Text, Status: s.Status})
	}
	c.emit(rulePlan, g.end, 0, ledger.KindPlan, g.id, "", ledger.Plan{Op: ledger.PlanReplace, Items: items})
}

// ending is how the turn ended: failed by a notice or its last reply's status,
// interrupted, or done; one that recorded no end ends interrupted.
func (g *group) ending() (ledger.Kind, any) {
	if g.notice != nil {
		return ledger.KindTurnFailed, ledger.TurnFailed{Reason: reasonNotice, Detail: g.notice.Text}
	}
	interrupted := func(reason string) (ledger.Kind, any) {
		return ledger.KindTurnEnded, ledger.TurnEnded{Status: string(ledger.TurnStatusInterrupted), Reason: reason, RunnerID: g.runnerID}
	}
	if g.inferred != nil {
		return interrupted(kindInferred)
	}
	if !g.ended() {
		return interrupted(ledger.ReasonDaemonRestart)
	}
	last := g.replies[len(g.replies)-1]
	switch last.Status {
	case statusFailed:
		return ledger.KindTurnFailed, ledger.TurnFailed{Reason: statusFailed}
	case statusInterrupted, statusAbandoned:
		return interrupted(last.Status)
	}
	return ledger.KindTurnEnded, ledger.TurnEnded{
		Status: string(ledger.TurnStatusDone), RunnerID: g.runnerID, Effort: last.Effort, Diff: last.Diff,
	}
}

func endOf(r LegacyTurn) time.Time {
	if r.EndedAt != nil {
		return *r.EndedAt
	}
	return r.StartedAt
}

func firstProvider(rows []LegacyTurn) string {
	for _, r := range rows {
		if r.ProviderID != "" {
			return r.ProviderID
		}
	}
	return ""
}

func firstRunner(rows []LegacyTurn) string {
	for _, r := range rows {
		if r.RunnerID != "" {
			return r.RunnerID
		}
	}
	return ""
}
