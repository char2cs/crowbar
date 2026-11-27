package ledger

import (
	"cmp"
	"slices"
	"sort"
	"strings"
	"time"
)

const IndexComplete = -1

type AssistantText struct {
	MessageID string `json:"messageId"`
	Index     int    `json:"index"`
	Final     bool   `json:"final,omitempty"`
	Text      string `json:"text"`
	Phase     string `json:"phase,omitempty"`
	Reply     string `json:"reply,omitempty"`
}

type Reasoning struct {
	MessageID string `json:"messageId,omitempty"`
	Text      string `json:"text"`
}

func ReplyID(turnID string) string { return "final-" + turnID }

type MessageKey struct{ TurnID, Scope, MessageID string }

type MessageText struct {
	Key       MessageKey
	Text      string
	Settled   bool
	FirstSeq  int64
	SettleSeq int64
	SettledAt time.Time
	Phase     string
	At        time.Time
}

type messageKeys struct {
	turnID  string
	indexes []int
	whole   string
}

func (s State) MessageTurn(scope, messageID string) (string, bool) {
	m, ok := s.keys.messages[ref(scope, messageID)]
	return m.turnID, ok
}

func (s State) HasChunk(scope, messageID string, index int) bool {
	return slices.Contains(s.keys.messages[ref(scope, messageID)].indexes, index)
}

func (s State) LastMessage(turnID string) (string, bool) {
	id, ok := s.keys.lastMain[turnID]
	return id, ok
}

func (s State) Reply(turnID string) (messageID, text string, ok bool) {
	id := ReplyID(turnID)
	if !s.HasChunk("", id, IndexComplete) {
		return "", "", false
	}
	return id, s.WholeText("", id), true
}

func (s State) ReplyBefore(turnID string) (prev, reply string, ok bool) {
	if _, spoke := s.keys.lastMain[turnID]; spoke {
		return "", "", false
	}
	turns := s.keys.turns
	i := slices.IndexFunc(turns, func(t keptTurn) bool { return t.id == turnID })
	if i < 1 || turns[i-1].runnerID != turns[i].runnerID {
		return "", "", false
	}
	prev = turns[i-1].id
	id := ReplyID(prev)
	if s.keys.lastMain[prev] != id || !s.HasChunk("", id, IndexComplete) {
		return "", "", false
	}
	return prev, s.WholeText("", id), true
}

func (s State) WholeText(scope, messageID string) string {
	return s.keys.messages[ref(scope, messageID)].whole
}

func (s *State) applyAssistantText(r Row) {
	body, err := DecodeBody[AssistantText](r)
	if err != nil || body.MessageID == "" {
		return
	}
	k := s.keys.init()
	key := ref(r.Scope, body.MessageID)
	m, seen := k.messages[key]
	if !seen {
		m.turnID = r.TurnID
		if r.Scope == "" {
			k.lastMain[r.TurnID] = body.MessageID
		}
	}
	if !slices.Contains(m.indexes, body.Index) {
		m.indexes = append(slices.Clip(m.indexes), body.Index)
	}
	if body.Index == IndexComplete {
		m.whole = fuller(m.whole, body.Text)
	}
	k.messages[key] = m
}

type MessageFold struct {
	byKey   map[MessageKey]*message
	order   []*message
	ended   map[string]bool
	open    []*message
	joiners map[MessageKey][]*message
}

type chunk struct {
	text  string
	final bool
}

type message struct {
	MessageText
	reply     string
	chunks    map[int]chunk
	completes []string
	forced    bool
	late      bool
}

func (f *MessageFold) Apply(r Row) {
	if f.byKey == nil {
		f.byKey, f.ended, f.joiners = map[MessageKey]*message{}, map[string]bool{}, map[MessageKey][]*message{}
	}
	switch r.Kind {
	case KindAssistantText:
		f.applyText(r)
	case KindTurnEnded, KindTurnFailed:
		if r.Scope != "" || f.ended[r.TurnID] {
			return
		}
		f.ended[r.TurnID] = true
		f.settle(r, func(m *message) bool { return m.Key.TurnID == r.TurnID || m.late })
	case KindRunnerExited:
		f.settle(r, func(*message) bool { return true })
	case KindUserMessage, KindTurnStarted, KindPromptSettled, KindSessionBound, KindSessionLost, KindMarker,
		KindRunnerStarted, KindReasoning, KindToolCall, KindAsk, KindAskResolved, KindSubagent, KindPlan, KindMove, KindShell:
	}
}

func (f *MessageFold) applyText(r Row) {
	body, err := DecodeBody[AssistantText](r)
	if err != nil || body.MessageID == "" {
		return
	}
	key := MessageKey{TurnID: r.TurnID, Scope: r.Scope, MessageID: body.MessageID}
	m, ok := f.byKey[key]
	if !ok {
		m = &message{
			MessageText: MessageText{Key: key, FirstSeq: r.Seq, At: r.At},
			reply:       body.Reply, chunks: map[int]chunk{}, late: f.ended[r.TurnID],
		}
		f.byKey[key] = m
		f.order, f.open = append(f.order, m), append(f.open, m)
		if m.reply != "" {
			target := MessageKey{TurnID: key.TurnID, Scope: key.Scope, MessageID: m.reply}
			f.joiners[target] = append(f.joiners[target], m)
		}
	}
	if body.Index == IndexComplete {
		m.completes = append(m.completes, body.Text)
		m.Phase = cmp.Or(body.Phase, m.Phase)
	} else if _, dup := m.chunks[body.Index]; !dup {
		m.chunks[body.Index] = chunk{text: body.Text, final: body.Final}
	}
	f.update(m, r)
}

func (f *MessageFold) settle(r Row, which func(*message) bool) {
	open := f.open[:0]
	for _, m := range f.open {
		if !m.Settled && which(m) {
			m.forced = true
			f.update(m, r)
		}
		if !m.Settled {
			open = append(open, m)
		}
	}
	clear(f.open[len(open):])
	f.open = open
}

func (f *MessageFold) update(m *message, r Row) {
	text, natural := m.text()
	settled := natural || m.forced
	if settled && (!m.Settled || text != m.Text) {
		m.SettleSeq, m.SettledAt = r.Seq, r.At
	}
	m.Text, m.Settled = text, settled
}

func (m *message) text() (string, bool) {
	stream, final := m.stream()
	text := ""
	if final {
		text = stream
	}
	for _, c := range m.completes {
		text = fuller(text, c)
	}
	if final {
		return text, true
	}
	if text == "" {
		return m.present(), len(m.completes) > 0
	}
	return fuller(text, stream), true
}

func (m *message) stream() (string, bool) {
	var b strings.Builder
	for i := 0; ; i++ {
		c, ok := m.chunks[i]
		if !ok {
			return b.String(), false
		}
		b.WriteString(c.text)
		if c.final {
			return b.String(), true
		}
	}
}

func (m *message) present() string {
	indexes := make([]int, 0, len(m.chunks))
	for i := range m.chunks {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	var b strings.Builder
	for _, i := range indexes {
		b.WriteString(m.chunks[i].text)
	}
	return b.String()
}

func fuller(text, next string) string {
	if text == "" || (len(next) > len(text) && strings.HasPrefix(next, text)) {
		return next
	}
	return text
}

func Related(a, b string) bool {
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func (f *MessageFold) Messages() []MessageText {
	out := make([]MessageText, 0, len(f.order))
	for _, m := range f.order {
		if _, joined := f.joins(m); !joined {
			out = append(out, f.shown(m))
		}
	}
	return out
}

func (f *MessageFold) joins(m *message) (*message, bool) {
	if m.reply == "" {
		return nil, false
	}
	target, ok := f.byKey[MessageKey{TurnID: m.Key.TurnID, Scope: m.Key.Scope, MessageID: m.reply}]
	if !ok || target == m || !Related(m.Text, target.Text) {
		return nil, false
	}
	return target, true
}

func (f *MessageFold) shown(m *message) MessageText {
	out := m.MessageText
	for _, j := range f.joiners[m.Key] {
		if target, ok := f.joins(j); !ok || target != m || !j.Settled {
			continue
		}
		if text := fuller(out.Text, j.Text); text != out.Text {
			out.Text, out.SettleSeq = text, max(out.SettleSeq, j.SettleSeq)
			out.SettledAt = latest(out.SettledAt, j.SettledAt)
		}
	}
	return out
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
