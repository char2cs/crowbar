// Package transcript projects a chat's ledger rows into the read shapes its
// clients already speak: messages, tool calls, subagents, interruptions and
// choices. It holds nothing the rows do not say.
package transcript

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// cachedChats is how many chats' rows the Projector keeps.
const cachedChats = 64

// Activity is a chat's record in the old read shapes: every listed message as
// a turn, then what the agent did around them.
type Activity struct {
	Turns         []domain.ActivityTurn
	ToolCalls     []domain.ActivityToolCall
	Subagents     []domain.ActivitySubagent
	Interruptions []domain.ActivityInterruption
	Choices       []Choice
}

// Projector keeps the folded rows of the chats read lately, advanced by the
// ledger's publication and refilled under the chat's writer on a miss, so no
// fill can install rows a publication already moved past.
type Projector struct {
	store *ledgerstore.Store
	mu    sync.Mutex
	chats map[string]*entry
	reads uint64 // counts reads: the chat read least lately is the one dropped
}

type entry struct {
	facts    *facts
	view     *view // nil until read since the last row
	lastRead uint64
}

// New builds the Projector over store; subscribe Apply to its publication.
func New(store *ledgerstore.Store) *Projector {
	return &Projector{store: store, chats: map[string]*entry{}}
}

// Apply advances chatID's cached rows by a publication. A chat it does not
// hold is left to its next read; one whose rows do not follow is dropped. It
// returns the listed messages the rows changed, for a cached chat.
func (p *Projector) Apply(chatID string, rows []ledger.Row) []domain.LedgerMessage {
	if len(rows) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.chats[chatID]
	if !ok {
		return nil
	}
	if rows[0].Seq != e.facts.head+1 {
		delete(p.chats, chatID)
		return nil
	}
	before := e.read().messages
	for _, r := range rows {
		e.facts.apply(r)
	}
	e.view = nil
	return changed(before, e.read().messages)
}

// Forget drops chatID: the chat is being erased.
func (p *Projector) Forget(chatID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.chats, chatID)
}

// Messages pages chatID's listed messages by their Sequence.
func (p *Projector) Messages(ctx context.Context, chatID string, after, before, limit int) (domain.LedgerPage, error) {
	v, err := p.view(ctx, chatID)
	if err != nil {
		return domain.LedgerPage{}, err
	}
	return page(v.messages, after, before, limit), nil
}

// ChatTurns is every listed message of chatID, in the order it is shown.
func (p *Projector) ChatTurns(ctx context.Context, chatID string) ([]domain.LedgerTurn, error) {
	v, err := p.view(ctx, chatID)
	if err != nil {
		return nil, err
	}
	shown := slices.SortedStableFunc(slices.Values(v.messages), func(a, b domain.LedgerMessage) int {
		return cmp.Or(cmp.Compare(a.DisplayOrder, b.DisplayOrder), cmp.Compare(a.Sequence, b.Sequence))
	})
	out := make([]domain.LedgerTurn, 0, len(shown))
	for _, m := range shown {
		out = append(out, m.LedgerTurn)
	}
	return out, nil
}

// Activity is chatID's whole record in the old read shapes.
func (p *Projector) Activity(ctx context.Context, chatID string) (Activity, error) {
	v, err := p.view(ctx, chatID)
	if err != nil {
		return Activity{}, err
	}
	return v.activity, nil
}

// PendingChoices are chatID's asks nothing has closed.
func (p *Projector) PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error) {
	v, err := p.view(ctx, chatID)
	if err != nil {
		return nil, err
	}
	var out []domain.ActivityChoice
	for _, c := range v.activity.Choices {
		if c.Pending() {
			out = append(out, c.ActivityChoice)
		}
	}
	return out, nil
}

func (p *Projector) view(ctx context.Context, chatID string) (*view, error) {
	if v, ok := p.cached(chatID); ok {
		return v, nil
	}
	var v *view
	// Under the writer no publication lands, so the rows read are the head.
	err := p.store.View(ctx, chatID, func(ledger.State) error {
		rows, err := p.store.Read(ctx, chatID, 0, 0)
		if err != nil {
			return err
		}
		v = p.install(chatID, rows)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("transcript: %s: %w", chatID, err)
	}
	return v, nil
}

func (p *Projector) cached(chatID string) (*view, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.chats[chatID]
	if !ok {
		return nil, false
	}
	p.touchLocked(e)
	return e.read(), true
}

func (p *Projector) install(chatID string, rows []ledger.Row) *view {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := &entry{facts: fold(chatID, rows)}
	p.touchLocked(e)
	p.chats[chatID] = e
	for len(p.chats) > cachedChats {
		oldest := chatID
		for id, c := range p.chats {
			if c.lastRead < p.chats[oldest].lastRead {
				oldest = id
			}
		}
		delete(p.chats, oldest)
	}
	return e.read()
}

func (p *Projector) touchLocked(e *entry) {
	p.reads++
	e.lastRead = p.reads
}

func (e *entry) read() *view {
	if e.view == nil {
		e.view = e.facts.view()
	}
	return e.view
}

// changed are the messages of after whose text or settling differs from before.
func changed(before, after []domain.LedgerMessage) []domain.LedgerMessage {
	seen := make(map[string]domain.LedgerMessage, len(before))
	for _, m := range before {
		seen[m.ID] = m
	}
	var out []domain.LedgerMessage
	for _, m := range after {
		if old, ok := seen[m.ID]; !ok || old.Text != m.Text || old.Sequence != m.Sequence {
			out = append(out, m)
		}
	}
	return out
}

// view is a chat's facts in the read shapes.
type view struct {
	messages []domain.LedgerMessage
	activity Activity
}

func (f *facts) view() *view {
	messages, turns := f.messages()
	return &view{
		messages: messages,
		activity: Activity{
			Turns: turns, ToolCalls: f.toolCalls(), Subagents: f.subagents(),
			Interruptions: f.interruptions(), Choices: f.choices(),
		},
	}
}

// facts is a chat's rows folded, one row at a time, into what the read
// shapes are built from.
type facts struct {
	chatID string
	head   int64
	turns  map[string]*turnFacts
	users  []userFact
	notes  []noticeFact
	text   ledger.MessageFold
	tools  map[scoped]*toolFact
	order  []*toolFact
	scopes map[string]*scopeFact
	spawns []*scopeFact
	marks  []*markFact
	// compacting is the compaction whose end has not been folded.
	compacting *markFact
	asks       map[string]*askFact
	asked      []*askFact
	exits      []ledger.Row
	// runners are the providers the chat's runners ran, by runner id.
	runners map[string]string
}

func fold(chatID string, rows []ledger.Row) *facts {
	f := &facts{
		chatID: chatID, turns: map[string]*turnFacts{}, tools: map[scoped]*toolFact{},
		scopes: map[string]*scopeFact{}, asks: map[string]*askFact{}, runners: map[string]string{},
	}
	for _, r := range rows {
		f.apply(r)
	}
	return f
}

// apply folds r, the row after every one already folded.
func (f *facts) apply(r ledger.Row) {
	f.head = r.Seq
	f.text.Apply(r)
	switch r.Kind {
	case ledger.KindUserMessage:
		f.applyUser(r)
	case ledger.KindTurnStarted:
		f.applyStart(r)
	case ledger.KindTurnEnded, ledger.KindTurnFailed:
		f.applyEnd(r)
	case ledger.KindRunnerStarted:
		if body, err := ledger.DecodeBody[ledger.RunnerStarted](r); err == nil {
			f.runners[body.RunnerID] = body.Provider
		}
	case ledger.KindRunnerExited:
		f.exits = append(f.exits, r)
	case ledger.KindPlan:
		f.applyPlan(r)
	case ledger.KindToolCall:
		f.applyTool(r)
	case ledger.KindSubagent:
		f.applySubagent(r)
	case ledger.KindMarker:
		f.applyMarker(r)
	case ledger.KindAsk:
		f.applyAsk(r)
	case ledger.KindAskResolved:
		f.applyAskResolved(r)
	case ledger.KindPromptSettled, ledger.KindSessionBound, ledger.KindSessionLost,
		ledger.KindAssistantText, ledger.KindReasoning:
		// f.text folds what was said; the rest the transcript does not show
	}
}

func (f *facts) turn(id string) *turnFacts {
	t, ok := f.turns[id]
	if !ok {
		t = &turnFacts{}
		f.turns[id] = t
	}
	return t
}

// closedBy is when what turnID's turn opened at seq is over though nothing
// closed it: at its turn's end, else at the first runner exit after it. A
// late fact of a turn that already ended is over with it, unless strict.
func (f *facts) closedBy(turnID string, seq int64, strict bool) (time.Time, bool) {
	if t, ok := f.turns[turnID]; ok && t.ended && (!strict || t.endSeq > seq) {
		return t.endAt, true
	}
	for _, x := range f.exits {
		if x.Seq > seq {
			return x.At, true
		}
	}
	return time.Time{}, false
}
