package conversation

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// turnsPage is how many rows a read of the chat's turns takes at a time.
const turnsPage = 512

// ChatTurns is every message of the chat, in the order it is shown: a
// person's once its turn started, the agent's once settled, each failed turn
// as a notice.
func (c *Conversations) ChatTurns(ctx context.Context, chatID string) ([]domain.LedgerTurn, error) {
	f := newTurnFold()
	for after := int64(0); ; {
		rows, err := c.ledger.Read(ctx, chatID, after, turnsPage)
		if err != nil {
			return nil, fmt.Errorf("agent: chat turns: %w", err)
		}
		for _, r := range rows {
			f.apply(r)
		}
		if len(rows) < turnsPage {
			return f.said(), nil
		}
		after = rows[len(rows)-1].Seq
	}
}

// noteSpeaker is who a note Crowbar adds to the conversation is shown from.
const noteSpeaker = "crowbar"

// turnFold is a chat's rows folded, one at a time, into the messages it said.
type turnFold struct {
	text    ledger.MessageFold
	turns   map[string]*turnFact
	runners map[string]string // provider by runner id
	users   []rowFact[ledger.UserMessage]
	notices []rowFact[string]
}

type turnFact struct {
	start           ledger.TurnStarted
	started, failed bool
	startSeq        int64
	startAt         time.Time
	effort          string
}

type rowFact[B any] struct {
	seq    int64
	at     time.Time
	turnID string
	body   B
}

// shown is a message at its place: order is where it is shown, seq the row
// that said or settled it.
type shown struct {
	order, seq int64
	turn       domain.LedgerTurn
}

func newTurnFold() *turnFold {
	return &turnFold{turns: map[string]*turnFact{}, runners: map[string]string{}}
}

func (f *turnFold) turn(id string) *turnFact {
	t, ok := f.turns[id]
	if !ok {
		t = &turnFact{}
		f.turns[id] = t
	}
	return t
}

// apply folds r, the row after every one already folded. Only the main
// agent's rows are the conversation; a subagent's are its own.
func (f *turnFold) apply(r ledger.Row) {
	f.text.Apply(r)
	if r.Scope != "" {
		return
	}
	switch r.Kind {
	case ledger.KindUserMessage:
		if body, err := ledger.DecodeBody[ledger.UserMessage](r); err == nil {
			f.users = append(f.users, rowFact[ledger.UserMessage]{seq: r.Seq, at: r.At, turnID: r.TurnID, body: body})
		}
	case ledger.KindTurnStarted:
		if body, err := ledger.DecodeBody[ledger.TurnStarted](r); err == nil {
			if t := f.turn(r.TurnID); !t.started {
				t.start, t.started, t.startSeq, t.startAt = body, true, r.Seq, r.At
			}
		}
	case ledger.KindTurnFailed:
		f.applyFailed(r)
	case ledger.KindTurnEnded:
		if body, err := ledger.DecodeBody[ledger.TurnEnded](r); err == nil {
			f.end(f.turn(r.TurnID), body)
		}
	case ledger.KindRunnerStarted:
		if body, err := ledger.DecodeBody[ledger.RunnerStarted](r); err == nil {
			f.runners[body.RunnerID] = body.Provider
		}
	case ledger.KindPromptSettled, ledger.KindSessionBound, ledger.KindSessionLost, ledger.KindMarker,
		ledger.KindRunnerExited, ledger.KindAssistantText, ledger.KindReasoning, ledger.KindToolCall,
		ledger.KindAsk, ledger.KindAskResolved, ledger.KindSubagent, ledger.KindPlan, ledger.KindMove:
		// the message fold reads the text; the rest says no message
	}
}

// applyFailed records a turn's first failure as a notice of its own.
func (f *turnFold) applyFailed(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.TurnFailed](r)
	if err != nil {
		return
	}
	t := f.turn(r.TurnID)
	if !t.failed {
		f.notices = append(f.notices, rowFact[string]{
			seq: r.Seq, at: r.At, turnID: r.TurnID, body: cmp.Or(body.Detail, body.Reason),
		})
	}
	t.failed = true
	f.end(t, ledger.TurnEnded{})
}

func (f *turnFold) end(t *turnFact, body ledger.TurnEnded) {
	t.effort = cmp.Or(body.Effort, t.effort)
	if !t.started && t.start.RunnerID == "" && body.RunnerID != "" {
		// A turn whose start never came is its ending runner's.
		t.start.RunnerID, t.start.Provider = body.RunnerID, f.runners[body.RunnerID]
	}
}

// said is every message said so far, in the order it is shown.
func (f *turnFold) said() []domain.LedgerTurn {
	var items []shown
	for _, u := range f.users {
		if it, ok := f.userTurn(u); ok {
			items = append(items, it)
		}
	}
	for _, m := range f.text.Messages() {
		if m.Key.Scope == "" && m.Settled && m.Text != "" {
			items = append(items, f.agentTurn(m))
		}
	}
	for _, n := range f.notices {
		items = append(items, shown{order: n.seq, seq: n.seq, turn: f.ledgerTurn(n.turnID, domain.LedgerTurn{
			ID: "notice-" + n.turnID, Role: domain.TurnRoleNotice, Text: n.body, At: n.at,
		})})
	}
	slices.SortStableFunc(items, func(a, b shown) int {
		return cmp.Or(cmp.Compare(a.order, b.order), cmp.Compare(a.seq, b.seq))
	})
	out := make([]domain.LedgerTurn, 0, len(items))
	for _, it := range items {
		out = append(out, it.turn)
	}
	return out
}

// userTurn is a prompt as said: a Crowbar prompt once its turn took it, where
// that turn started and under the request it was sent with.
func (f *turnFold) userTurn(u rowFact[ledger.UserMessage]) (shown, bool) {
	lt := domain.LedgerTurn{ID: u.turnID, Role: domain.TurnRoleUser, Text: u.body.Text, At: u.at}
	seq := u.seq
	switch u.body.Origin {
	case ledger.OriginCrowbar:
		t := f.turns[u.turnID]
		if t == nil || !t.started {
			return shown{}, false
		}
		lt.ID, lt.At, seq = u.body.RequestID, t.startAt, t.startSeq
	case ledger.OriginHarness:
		lt.Role = domain.TurnRoleHarness
	}
	lt = f.ledgerTurn(u.turnID, lt)
	if u.body.Origin == ledger.OriginNote {
		lt.Provider = noteSpeaker
	}
	return shown{order: seq, seq: seq, turn: lt}, true
}

func (f *turnFold) agentTurn(m ledger.MessageText) shown {
	lt := f.ledgerTurn(m.Key.TurnID, domain.LedgerTurn{
		ID: "msg-" + m.Key.MessageID, Role: domain.TurnRoleAssistant, Text: m.Text, At: m.At,
	})
	if t := f.turns[m.Key.TurnID]; t != nil {
		lt.Effort = t.effort
	}
	return shown{order: m.FirstSeq, seq: m.SettleSeq, turn: lt}
}

// ledgerTurn is lt said in turnID's turn, by the provider and runner it ran on.
func (f *turnFold) ledgerTurn(turnID string, lt domain.LedgerTurn) domain.LedgerTurn {
	if t := f.turns[turnID]; t != nil {
		lt.Provider, lt.RunnerID, lt.SessionID = t.start.Provider, t.start.RunnerID, t.start.SessionID
	}
	return lt
}
