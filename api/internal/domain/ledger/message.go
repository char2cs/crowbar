package ledger

import (
	"cmp"
	"slices"
	"sort"
	"strings"
	"time"
)

// IndexComplete is the index of an assistant_text row that carries its
// message's whole text rather than one chunk of it.
const IndexComplete = -1

// AssistantText is the body of an assistant_text row: chunk Index of a
// message, or with IndexComplete the message's whole text.
type AssistantText struct {
	MessageID string `json:"messageId"`
	Index     int    `json:"index"`
	Final     bool   `json:"final,omitempty"`
	Text      string `json:"text"`
	Phase     string `json:"phase,omitempty"`
	// Reply names the reply a turn closed on before anything of it had
	// streamed: the message joins it when one text extends the other.
	Reply string `json:"reply,omitempty"`
}

// Reasoning is the body of a reasoning row: what the model thought on its way
// to a message, recorded once when its turn ends.
type Reasoning struct {
	MessageID string `json:"messageId,omitempty"`
	Text      string `json:"text"`
}

// ReplyID is the message id a turn's end records its reply under when no
// message of the turn had streamed.
func ReplyID(turnID string) string { return "final-" + turnID }

// MessageKey names one message: its turn, its scope and its own id.
type MessageKey struct{ TurnID, Scope, MessageID string }

// MessageText is one message as its rows fold.
type MessageText struct {
	Key  MessageKey
	Text string
	// Settled: its stream is complete, a complete row exists, or an end
	// after it began (its turn's, any turn's for a late message, a runner's).
	Settled   bool
	FirstSeq  int64     // seq of its first row
	SettleSeq int64     // seq of the row that last settled or changed its settled text
	SettledAt time.Time // time of that row
	Phase     string
	At        time.Time // time of its first row
}

// messageKeys is what a retried chunk of one message is recognised by.
type messageKeys struct {
	turnID, reply string
	indexes       []int
}

// reply is the reply a turn's end recorded while nothing of it had streamed.
type reply struct{ messageID, text string }

// MessageTurn is the turn the message messageID of scope was first recorded in.
func (s State) MessageTurn(scope, messageID string) (string, bool) {
	m, ok := s.keys.messages[ref(scope, messageID)]
	return m.turnID, ok
}

// ShownAs is the id the message is shown under: the reply it joins, else its own.
func (s State) ShownAs(scope, messageID string) string {
	return cmp.Or(s.keys.messages[ref(scope, messageID)].reply, messageID)
}

// HasChunk reports whether chunk index of the message is already recorded.
func (s State) HasChunk(scope, messageID string, index int) bool {
	return slices.Contains(s.keys.messages[ref(scope, messageID)].indexes, index)
}

// LastMessage is turnID's newest main-scope message id.
func (s State) LastMessage(turnID string) (string, bool) {
	id, ok := s.keys.lastMain[turnID]
	return id, ok
}

// Reply is the reply turnID's end recorded while nothing of it had streamed.
func (s State) Reply(turnID string) (messageID, text string, ok bool) {
	r, ok := s.keys.replies[turnID]
	return r.messageID, r.text, ok
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
		m.turnID, m.reply = r.TurnID, body.Reply
		if r.Scope == "" {
			k.lastMain[r.TurnID] = body.MessageID
		}
	}
	if !slices.Contains(m.indexes, body.Index) {
		m.indexes = append(slices.Clip(m.indexes), body.Index)
	}
	k.messages[key] = m
	if body.Index == IndexComplete && body.MessageID == ReplyID(r.TurnID) {
		k.replies[r.TurnID] = reply{messageID: body.MessageID, text: body.Text}
	}
}

// MessageFold folds assistant_text rows one committed row at a time. A text is
// the contiguous stream 0..k when chunk k is final; else the complete row; else
// the chunks present, in index order. A text that extends the one chosen wins.
type MessageFold struct {
	byKey map[MessageKey]*message
	order []*message
	ended map[string]bool // main turns whose end is folded
	// open are the messages not yet settled; an end settles only these.
	open []*message
	// joiners are the messages naming each reply, by the reply's key.
	joiners map[MessageKey][]*message
	// touched are the messages whose text or settling moved since Changed.
	touched []*message
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
	forced    bool // settled by an end rather than by its own rows
	late      bool // began after its turn had ended
	touched   bool // in the fold's touched
}

// Apply folds r, which must follow every row already folded.
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
		KindRunnerStarted, KindReasoning, KindToolCall, KindAsk, KindAskResolved, KindSubagent, KindPlan:
		// neither says nor settles a message
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

// settle settles, at r, every unsettled message which says.
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
	if (text != m.Text || settled != m.Settled) && !m.touched {
		m.touched, f.touched = true, append(f.touched, m)
	}
	m.Text, m.Settled = text, settled
}

// text is the message's text and whether its own rows settle it.
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

// stream is the chunks contiguous from 0, and whether they run through the
// final one.
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

// present is every chunk recorded, in index order, gaps and all.
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

// fuller is next when it extends text, else text; next fills an empty text.
func fuller(text, next string) string {
	if text == "" || (len(next) > len(text) && strings.HasPrefix(next, text)) {
		return next
	}
	return text
}

// Related reports whether one text extends the other.
func Related(a, b string) bool {
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// Messages is every message by its first row; a late message that agrees with
// the reply its turn closed on is shown as that reply, once it settles.
func (f *MessageFold) Messages() []MessageText {
	out := make([]MessageText, 0, len(f.order))
	for _, m := range f.order {
		if _, joined := f.joins(m); !joined {
			out = append(out, f.shown(m))
		}
	}
	return out
}

// Changed is, as Messages shows them, the messages the rows folded since the
// last Changed moved, with the replies they join and the messages joining
// them; a message shown as its reply is left out.
func (f *MessageFold) Changed() []MessageText {
	var moved []*message
	seen := map[*message]bool{}
	add := func(m *message) {
		if !seen[m] {
			seen[m], moved = true, append(moved, m)
		}
	}
	for _, m := range f.touched {
		m.touched = false
		add(m)
		if target, ok := f.joins(m); ok {
			add(target)
		}
		for _, j := range f.joiners[m.Key] {
			add(j)
		}
	}
	clear(f.touched)
	f.touched = f.touched[:0]
	slices.SortFunc(moved, func(a, b *message) int { return cmp.Compare(a.FirstSeq, b.FirstSeq) })
	out := make([]MessageText, 0, len(moved))
	for _, m := range moved {
		if _, joined := f.joins(m); !joined {
			out = append(out, f.shown(m))
		}
	}
	return out
}

// joins is the reply m is shown as: the one it names, when they agree. An
// unsettled one is shown under the reply's id until it settles.
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

// shown is m with the settled messages that join it.
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
