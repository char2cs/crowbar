package chatledger

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Result is one legacy chat as ledger rows: At is the legacy record's time,
// Seq is 1..n in order. Skipped names each legacy record not converted, and why.
type Result struct {
	Rows    []ledger.Row
	Skipped []string
}

// Owner answers which chat already holds a (provider, session) binding.
type Owner func(provider, sessionID string) (chatID string, ok bool)

// ErrNotAtRest is a conversion that would leave the chat with work open.
var ErrNotAtRest = errors.New("chatledger: the converted chat is not at rest")

// What a row restates, in the order rows of one instant are recorded: a turn
// opens before its activity and ends after it.
const (
	ruleSession = iota
	ruleTurnStart
	ruleText
	ruleTool
	ruleSubagent
	ruleMarker
	ruleAsk
	rulePlan
	ruleTurnEnd
	rulePrompt
	ruleRunner
)

// ConvertChat restates in's records as ledger rows. It is pure: the same
// input and owner give the same rows, ids included.
func ConvertChat(in LegacyChat, owner Owner) (Result, error) {
	c := &conversion{chatID: in.ChatID, skipped: slices.Clone(in.Unreadable)}
	c.bindSessions(in, owner)
	c.turns(in)
	c.subagents(in.Subagents)
	c.tools(in.Tools)
	c.interruptions(in.Interruptions)
	c.choices(in.Choices)
	c.prompts(in.Prompts)
	c.runners(in.Runners)
	if c.err != nil {
		return Result{}, fmt.Errorf("chatledger: convert %s: %w", in.ChatID, c.err)
	}
	rows, err := c.ordered()
	if err != nil {
		return Result{}, err
	}
	return Result{Rows: rows, Skipped: c.skipped}, nil
}

// entry is a row and what orders it among the chat's others.
type entry struct {
	row       ledger.Row
	rule      int
	legacySeq int64
	n         int
}

type conversion struct {
	chatID  string
	out     []entry
	skipped []string
	err     error

	current map[string]string // provider → the session the chat is bound to on it
	bound   map[string]bool   // provider+"\x00"+session bound to this chat
	groups  []*group
	byID    map[string]*group // legacy turn id → its ledger turn
	agents  map[string]*group // legacy subagent id → its ledger turn
	adopted map[string]bool   // runners the chat is left live on
	latest  time.Time
}

func (c *conversion) emit(rule int, at time.Time, legacySeq int64, kind ledger.Kind, turnID, scope string, body any) {
	r, err := ledger.NewRow(kind, turnID, scope, body)
	if err != nil {
		c.err = errors.Join(c.err, err)
		return
	}
	r.At = micros(at)
	if r.At.After(c.latest) {
		c.latest = r.At
	}
	c.out = append(c.out, entry{row: r, rule: rule, legacySeq: legacySeq, n: len(c.out)})
}

func (c *conversion) skipf(format string, args ...any) {
	c.skipped = append(c.skipped, fmt.Sprintf(format, args...))
}

// ordered numbers the rows by time, then rule, then legacy seq; legacy seqs of
// different tables were minted at different moments, so they only break ties.
// Every row must pass Check and the whole must fold to a chat at rest.
func (c *conversion) ordered() ([]ledger.Row, error) {
	slices.SortStableFunc(c.out, func(a, b entry) int {
		return cmp.Or(a.row.At.Compare(b.row.At), cmp.Compare(a.rule, b.rule),
			cmp.Compare(a.legacySeq, b.legacySeq), cmp.Compare(a.n, b.n))
	})
	rows := make([]ledger.Row, 0, len(c.out))
	st := ledger.State{ChatID: c.chatID}
	for i, e := range c.out {
		r := e.row
		r.ChatID, r.Seq = c.chatID, int64(i+1)
		if err := ledger.Check(st, r); err != nil {
			return nil, fmt.Errorf("chatledger: convert %s: row %d: %w", c.chatID, r.Seq, err)
		}
		st.Apply(r)
		rows = append(rows, r)
	}
	if err := AtRest(st, c.adopted); err != nil {
		return nil, fmt.Errorf("chatledger: convert %s: %w", c.chatID, err)
	}
	return rows, nil
}

// AtRest refuses a chat with a turn running, a prompt pending or a subagent
// live, and one left live on any runner but those the boot reconcile ends.
func AtRest(st ledger.State, adopted map[string]bool) error {
	if t, ok := st.RunningTurn(); ok {
		return fmt.Errorf("%w: turn %s runs", ErrNotAtRest, t.ID)
	}
	if p := st.PendingPrompts(); len(p) > 0 {
		return fmt.Errorf("%w: prompt %s is pending", ErrNotAtRest, p[0].RequestID)
	}
	if a := st.Subagents(); len(a) > 0 {
		return fmt.Errorf("%w: subagent %s is live", ErrNotAtRest, a[0].Scope)
	}
	if r, ok := st.LiveRunner(); ok && !adopted[r.RunnerID] {
		return fmt.Errorf("%w: runner %s is live", ErrNotAtRest, r.RunnerID)
	}
	return nil
}

// turnID is the ledger turn a legacy record opens: the same on every run.
func turnID(chatID, legacyID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("crowbar:layout1:"+chatID+":"+legacyID)).String()
}

// micros is t as the ledger store keeps it.
func micros(t time.Time) time.Time { return time.UnixMicro(t.UnixMicro()).UTC() }

// earliest is the earliest of ts that is set; zero when none is.
func earliest(ts ...time.Time) time.Time {
	var out time.Time
	for _, t := range ts {
		if !t.IsZero() && (out.IsZero() || t.Before(out)) {
			out = t
		}
	}
	return out
}

func latest(ts ...time.Time) time.Time {
	var out time.Time
	for _, t := range ts {
		if t.After(out) {
			out = t
		}
	}
	return out
}
