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
	"sync/atomic"
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
// ledger's publication and refilled under the chat's writer on a miss. Each
// chat has its own lock: no chat's publication waits on another's.
type Projector struct {
	store *ledgerstore.Store
	chats sync.Map // chat id → *entry
	held  atomic.Int64
	reads atomic.Uint64 // counts reads: the chat read least lately is the one dropped
}

type entry struct {
	mu       sync.Mutex
	facts    *facts
	view     *view // nil until read since the last row
	lastRead atomic.Uint64
	dropped  atomic.Bool
}

// New builds the Projector over store; subscribe Apply to its publication.
func New(store *ledgerstore.Store) *Projector {
	return &Projector{store: store}
}

// Apply folds a publication into chatID's cached rows at the cost of its rows,
// returning the agent's listed messages they settled or changed. An uncached
// chat waits for its next read; one whose rows do not follow is dropped.
func (p *Projector) Apply(chatID string, rows []ledger.Row) []domain.LedgerMessage {
	if len(rows) == 0 {
		return nil
	}
	e, ok := p.entry(chatID)
	if !ok {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dropped.Load() {
		return nil
	}
	if rows[0].Seq != e.facts.head+1 {
		p.drop(chatID, e)
		return nil
	}
	for _, r := range rows {
		e.facts.apply(r)
	}
	e.view = nil
	return e.facts.settled()
}

// Forget drops chatID: the chat is being erased.
func (p *Projector) Forget(chatID string) {
	if e, ok := p.entry(chatID); ok {
		p.drop(chatID, e)
	}
}

func (p *Projector) entry(chatID string) (*entry, bool) {
	v, ok := p.chats.Load(chatID)
	if !ok {
		return nil, false
	}
	e, ok := v.(*entry)
	return e, ok
}

// drop drops e, chatID's entry, and reports whether it was still held.
func (p *Projector) drop(chatID string, e *entry) bool {
	if !p.chats.CompareAndDelete(chatID, e) {
		return false
	}
	e.dropped.Store(true)
	p.held.Add(-1)
	return true
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
	e, ok := p.entry(chatID)
	if !ok {
		return nil, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dropped.Load() {
		return nil, false
	}
	e.lastRead.Store(p.reads.Add(1))
	return e.read(), true
}

// install caches chatID folded from rows, its ledger's head, then drops the
// chat read least lately while more than cachedChats are held.
func (p *Projector) install(chatID string, rows []ledger.Row) *view {
	e := &entry{facts: fold(chatID, rows)}
	e.lastRead.Store(p.reads.Add(1))
	v := e.read()
	if old, loaded := p.chats.Swap(chatID, e); !loaded {
		p.held.Add(1)
	} else if was, ok := old.(*entry); ok {
		was.dropped.Store(true)
	}
	for p.held.Load() > cachedChats {
		if !p.dropOldest() {
			break
		}
	}
	return v
}

// dropOldest drops the chat read least lately, and reports whether it did.
func (p *Projector) dropOldest() bool {
	oldestID, oldest := "", (*entry)(nil)
	p.chats.Range(func(k, v any) bool {
		id, _ := k.(string)
		if c, ok := v.(*entry); ok && (oldest == nil || c.lastRead.Load() < oldest.lastRead.Load()) {
			oldestID, oldest = id, c
		}
		return true
	})
	return oldest != nil && p.drop(oldestID, oldest)
}

func (e *entry) read() *view {
	if e.view == nil {
		e.view = e.facts.view()
	}
	return e.view
}

// settled are the agent's listed messages the rows folded since the last call
// settled or changed, in the order they are listed.
func (f *facts) settled() []domain.LedgerMessage {
	var out []domain.LedgerMessage
	for _, m := range f.text.Changed() {
		if m.Key.Scope == "" && m.Settled && m.Text != "" {
			out = append(out, f.agentMessage(m).msg)
		}
	}
	slices.SortStableFunc(out, func(a, b domain.LedgerMessage) int {
		return cmp.Or(cmp.Compare(a.Sequence, b.Sequence), cmp.Compare(a.DisplayOrder, b.DisplayOrder))
	})
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
	exits      []exitFact
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
	f.text.Changed() // what a later publication changed starts here
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
		if body, err := ledger.DecodeBody[ledger.RunnerExited](r); err == nil {
			f.exits = append(f.exits, exitFact{runnerID: body.RunnerID, seq: r.Seq, at: r.At})
		}
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

// exitFact is a runner's exit.
type exitFact struct {
	runnerID string
	seq      int64
	at       time.Time
}

// closedBy is when what turnID's turn opened at seq is over though nothing
// closed it: at its turn's end, else at its runner's first exit after it. A
// late fact of a turn that already ended is over with it, unless strict.
func (f *facts) closedBy(turnID string, seq int64, strict bool) (time.Time, bool) {
	t, ok := f.turns[turnID]
	if ok && t.ended && (!strict || t.endSeq > seq) {
		return t.endAt, true
	}
	return f.exitAfter(t, seq)
}

// exitAfter is the first exit after seq of t's runner; of any runner when t
// names none.
func (f *facts) exitAfter(t *turnFacts, seq int64) (time.Time, bool) {
	var runnerID string
	if t != nil {
		runnerID = t.start.RunnerID
	}
	for _, x := range f.exits {
		if x.seq > seq && (runnerID == "" || x.runnerID == runnerID) {
			return x.at, true
		}
	}
	return time.Time{}, false
}
