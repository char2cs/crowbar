package chatledger

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

type Result struct {
	Rows    []ledger.Row
	Skipped []string
	LeftOut int
}

type Owner func(provider, sessionID string) (chatID string, ok bool)

var errUnreadable = errors.New("records that cannot be read")

var ErrNotAtRest = errors.New("chatledger: the converted chat is not at rest")

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

func ConvertChat(in LegacyChat, owner Owner) (Result, error) {
	if len(in.Unreadable) > 0 && nothingRead(in) {
		return Result{}, fmt.Errorf("chatledger: convert %s: %w: %s", in.ChatID, errUnreadable, clip(strings.Join(in.Unreadable, "; "), noteBytes))
	}
	c := &conversion{
		chatID: in.ChatID, skipped: slices.Concat(in.Unreadable, in.Partial), leftOut: len(in.Unreadable), partial: len(in.Partial),
	}
	c.bindSessions(in, owner)
	c.turns(in)
	c.subagents(in.Subagents)
	c.tools(in.Tools)
	c.interruptions(in.Interruptions)
	c.choices(in.Choices)
	c.prompts(in.Prompts)
	c.markLeftOut()
	c.runners(in.Runners)
	if c.err != nil {
		return Result{}, fmt.Errorf("chatledger: convert %s: %w", in.ChatID, c.err)
	}
	rows, err := c.ordered()
	if err != nil {
		return Result{}, err
	}
	return Result{Rows: rows, Skipped: c.skipped, LeftOut: c.leftOut + c.partial}, nil
}

func nothingRead(in LegacyChat) bool {
	return len(in.Conversations)+len(in.Turns)+len(in.Tools)+len(in.Subagents)+len(in.Interruptions)+
		len(in.Choices)+len(in.Prompts)+len(in.Runners) == 0
}

const maxNamedRecords = 20

func leftOutNamed(skipped []string) []string {
	return slices.Clone(skipped[:min(len(skipped), maxNamedRecords)])
}

func (c *conversion) markLeftOut() {
	if c.leftOut+c.partial == 0 {
		return
	}
	what := inWords(c.leftOut) + " could not be converted"
	switch {
	case c.leftOut == 0 && c.partial == 1:
		what = "1 legacy record was converted only in part"
	case c.leftOut == 0:
		what = inWords(c.partial) + " were converted only in part"
	case c.partial > 0:
		what += fmt.Sprintf(", and %d more only in part", c.partial)
	}
	c.emit(ruleMarker, c.latest.Add(time.Microsecond), 0, ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerUnreadable, Detail: what, Records: leftOutNamed(c.skipped)})
}

func inWords(n int) string {
	if n == 1 {
		return "1 legacy record"
	}
	return fmt.Sprintf("%d legacy records", n)
}

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
	leftOut int
	partial int
	err     error

	current map[string]string
	bound   map[string]bool
	groups  []*group
	byID    map[string]*group
	bySeq   []seqGroup
	agents  map[string]*group
	adopted map[string]bool
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
	c.skipped = append(c.skipped, clip(fmt.Sprintf(format, args...), reasonBytes))
	c.leftOut++
}

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

func turnID(chatID, legacyID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("crowbar:layout1:"+chatID+":"+legacyID)).String()
}

func micros(t time.Time) time.Time { return time.UnixMicro(t.UnixMicro()).UTC() }

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
