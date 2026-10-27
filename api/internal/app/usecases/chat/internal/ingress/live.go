package ingress

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// The kinds of streamed text. The answer's own kind is "": the only one a row
// ever holds; the others are shown while they happen and never recorded.
const (
	KindAnswer     = ""
	KindReasoning  = "reasoning"
	KindToolOutput = "tool_output"
	KindDiff       = "diff"
)

// StreamAnswer is the answer's stream as a feed names it; the other streams
// are named by their kind.
const StreamAnswer = "answer"

// FeedDelta is one frame of a stream no row holds: only the text new since its
// stream's last frame, numbered per stream, or the whole text when Replace.
type FeedDelta struct {
	ID      string `json:"id"` // message id (answer, reasoning) or tool id (tool_output, diff)
	TurnID  string `json:"turnId"`
	Scope   string `json:"scope,omitempty"`
	Stream  string `json:"stream"` // "answer" | "reasoning" | "tool_output" | "diff"
	N       int    `json:"n"`      // 1-based position of this frame in its stream
	Text    string `json:"text"`   // the new suffix; everything so far when Replace
	Replace bool   `json:"replace,omitempty"`
}

// toolOutputWindow is how much of a tool's output a frame carries: it is only
// ever shown, and its whole text lands on the call's own result.
const toolOutputWindow = 64 << 10

// LiveDelta is one increment of streamed text no row holds: what the live
// frame of its stream is built from.
type LiveDelta struct {
	TurnID, Scope, ID, Kind string
	Index                   int
	// Numbered: the provider numbered the increment, so a row holds it too.
	Numbered bool
	// Replace: Text is the stream's whole text, not an increment of it.
	Replace bool
	Text    string
	// ShownAs is the id the frame names: the message's own, or the reply it joins.
	ShownAs string
	// Floor is the text a row already shows under ShownAs.
	Floor string
}

func (r *Recorder) unrecorded(chatID, runnerID string) Unrecorded {
	return Unrecorded{
		Answers:   r.live.unrecorded(chatID, runnerID, "", KindAnswer),
		Reasoning: r.live.unrecorded(chatID, runnerID, "", KindReasoning),
		Diff:      r.live.unrecorded(chatID, runnerID, "", KindDiff),
	}
}

// publishLive shows d once its rows are in, with the stream so far. A frame
// of a stream shown as a reply never shows less than the reply's own text.
func (r *Recorder) publishLive(chat domain.Chat, runnerID string, d LiveDelta) {
	text, startedAt, fed := r.live.observe(chat.ID, runnerID, d, time.Now())
	if fed != nil {
		r.frames.FeedDelta(chat.ID, *fed)
	}
	if d.Kind != KindAnswer {
		startedAt = time.Time{}
	}
	if d.Floor != "" && (len(text) <= len(d.Floor) || !strings.HasPrefix(text, d.Floor)) {
		text = d.Floor
	}
	r.frames.MessageDelta(chat.ID, chat.WorkspaceID, d.ShownAs, text, d.Kind, startedAt)
}

// Snapshot is one Replace frame per stream of chatID no row holds, each at its
// stream's current number, in the order they began.
func (r *Recorder) Snapshot(chatID string, st ledger.State) []FeedDelta {
	return r.live.snapshot(chatID, st)
}

// ExitRows are what runnerID's exit records of the answers it streamed with no
// row of their own; the caller commits them with the exit, then calls Retire.
func (r *Recorder) ExitRows(st ledger.State, chatID, runnerID string) ([]ledger.Row, error) {
	return Salvage(st, r.live.unrecorded(chatID, runnerID, "", KindAnswer))
}

// Retire drops runnerID's streamed text on chatID: its exit is recorded. Its
// exit interrupted what it was spawning, so what it held is dead-lettered.
func (r *Recorder) Retire(chatID, runnerID string) {
	r.live.forget(chatID, runnerID, "")
	r.unclaimed(context.Background(), chatID, r.unbound.releaseRunner(chatID, runnerID))
}

// liveKey names one stream: whose turn it belongs to, and which block of it.
type liveKey struct {
	runnerID, turnID, scope, id, kind string
}

type liveBlock struct {
	parts map[int]string
	next  int // the index an unnumbered increment takes: arrival order
	// numbered: the provider numbered its own increments, so a row holds them.
	numbered bool
	at       time.Time
	order    int // when it began among the chat's streams: the order it was said in
	dropped  int // the bytes of a tool output trimmed off its head
	n        int // the feed frames its stream has had
}

// live is the text streamed so far that no row holds, per chat, so every frame
// carries the whole text: a dropped frame then costs nothing. It is dropped
// when the turn it belongs to ends.
type live struct {
	mu     sync.Mutex
	byChat map[string]map[liveKey]*liveBlock
	began  int
}

func newLive() *live { return &live{byChat: map[string]map[liveKey]*liveBlock{}} }

// observe folds d into its stream: its whole text, when it began, and the feed
// frame of what it added. A numbered stream has none: its rows carry it.
func (l *live) observe(chatID, runnerID string, d LiveDelta, now time.Time) (string, time.Time, *FeedDelta) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := liveKey{runnerID: runnerID, turnID: d.TurnID, scope: d.Scope, id: d.ID, kind: d.Kind}
	b := l.blockLocked(chatID, k, now)
	before := b.join(d.Kind)
	b.fold(d)
	after := b.join(d.Kind)
	if d.Kind == KindToolOutput {
		b.trim()
	}
	text := b.text(d.Kind)
	if b.numbered || after == before {
		return text, b.at, nil
	}
	b.n++
	fed := k.feed(b.n, text, true)
	if strings.HasPrefix(after, before) {
		fed.Text, fed.Replace = after[len(before):], false
	}
	return text, b.at, &fed
}

// fold takes d into the block's parts: a numbered answer increment at its
// index, an unnumbered one next, other kinds appended to their index's part.
func (b *liveBlock) fold(d LiveDelta) {
	switch {
	case d.Replace:
		b.parts = map[int]string{0: d.Text}
	case d.Kind != KindAnswer:
		b.parts[d.Index] += d.Text
	case d.Numbered:
		b.numbered = true
		b.parts[d.Index] = d.Text
	default:
		b.parts[b.next] = d.Text
		b.next++
	}
}

func (k liveKey) feed(n int, text string, replace bool) FeedDelta {
	stream := k.kind
	if stream == KindAnswer {
		stream = StreamAnswer
	}
	return FeedDelta{ID: k.id, TurnID: k.turnID, Scope: k.scope, Stream: stream, N: n, Text: text, Replace: replace}
}

// snapshot is a Replace frame of each of chatID's streams no row holds.
func (l *live) snapshot(chatID string, st ledger.State) []FeedDelta {
	l.mu.Lock()
	defer l.mu.Unlock()
	blocks := l.byChat[chatID]
	keys := slices.Collect(maps.Keys(blocks))
	slices.SortFunc(keys, func(a, b liveKey) int { return blocks[a].order - blocks[b].order })
	var out []FeedDelta
	for _, k := range keys {
		if b := blocks[k]; !b.numbered && b.n > 0 && !k.recorded(st) {
			out = append(out, k.feed(b.n, b.text(k.kind), true))
		}
	}
	return out
}

// recorded reports whether a row in st holds the stream's whole text: a
// message's complete chunk, or a tool's result. Reasoning and the diff are
// the turn's, recorded at its end.
func (k liveKey) recorded(st ledger.State) bool {
	switch k.kind {
	case KindAnswer:
		return st.HasChunk(k.scope, k.id, ledger.IndexComplete)
	case KindToolOutput:
		return st.HasToolPhase(k.scope, k.id, ledger.ToolPost) || st.HasToolPhase(k.scope, k.id, ledger.ToolFail)
	}
	return false
}

// dropRecorded drops runnerID's streams on chatID a row in st now holds.
func (l *live) dropRecorded(chatID, runnerID string, st ledger.State) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k := range l.byChat[chatID] {
		if k.runnerID == runnerID && k.recorded(st) {
			delete(l.byChat[chatID], k)
		}
	}
	if len(l.byChat[chatID]) == 0 {
		delete(l.byChat, chatID)
	}
}

// trim keeps a tool output's tail once it is twice the window: each delta
// then costs at most the window, however long the stream runs.
func (b *liveBlock) trim() {
	whole := b.join(KindToolOutput)
	if len(whole) <= 2*toolOutputWindow {
		return
	}
	cut := len(whole) - toolOutputWindow
	for cut < len(whole) && !utf8.RuneStart(whole[cut]) {
		cut++
	}
	last := slices.Max(slices.Collect(maps.Keys(b.parts)))
	b.parts, b.dropped = map[int]string{last: whole[cut:]}, b.dropped+cut
}

func (l *live) blockLocked(chatID string, k liveKey, now time.Time) *liveBlock {
	blocks := l.byChat[chatID]
	if blocks == nil {
		blocks = map[liveKey]*liveBlock{}
		l.byChat[chatID] = blocks
	}
	b := blocks[k]
	if b == nil {
		l.began++
		b = &liveBlock{parts: map[int]string{}, at: now, order: l.began}
		blocks[k] = b
	}
	return b
}

// text is the stream's whole text; a tool output's is its last window, behind
// a marker saying how much came before.
func (b *liveBlock) text(kind string) string {
	whole := b.join(kind)
	if kind != KindToolOutput || len(whole) <= toolOutputWindow && b.dropped == 0 {
		return whole
	}
	cut := max(len(whole)-toolOutputWindow, 0)
	for cut < len(whole) && !utf8.RuneStart(whole[cut]) {
		cut++
	}
	return fmt.Sprintf("[… %d earlier bytes not shown]\n", b.dropped+cut) + whole[cut:]
}

func (b *liveBlock) join(kind string) string {
	indexes := make([]int, 0, len(b.parts))
	for i := range b.parts {
		indexes = append(indexes, i)
	}
	slices.Sort(indexes)
	parts := make([]string, 0, len(indexes))
	for _, i := range indexes {
		parts = append(parts, b.parts[i])
	}
	if kind == KindAnswer {
		return strings.Join(parts, "")
	}
	return strings.Join(parts, "\n\n")
}

// liveText is one unrecorded stream as a turn's end or a runner's exit reads it.
type liveText struct {
	TurnID, Scope, ID, Text string
}

// unrecorded is runnerID's answers no row holds and the turn's other text of
// kind, of turnID's turn when it is named.
func (l *live) unrecorded(chatID, runnerID, turnID, kind string) []liveText {
	l.mu.Lock()
	defer l.mu.Unlock()
	var keys []liveKey
	blocks := l.byChat[chatID]
	for k, b := range blocks {
		if k.runnerID == runnerID && k.kind == kind && (turnID == "" || k.turnID == turnID) && !b.numbered {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b liveKey) int { return blocks[a].order - blocks[b].order })
	var out []liveText
	for _, k := range keys {
		if text := blocks[k].text(kind); text != "" {
			out = append(out, liveText{TurnID: k.turnID, Scope: k.scope, ID: k.id, Text: text})
		}
	}
	return out
}

// forget drops runnerID's streams of turnID's turn ("" for every turn): what
// they said is recorded, or was only ever shown.
func (l *live) forget(chatID, runnerID, turnID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k := range l.byChat[chatID] {
		if k.runnerID == runnerID && (turnID == "" || k.turnID == turnID) {
			delete(l.byChat[chatID], k)
		}
	}
	if len(l.byChat[chatID]) == 0 {
		delete(l.byChat, chatID)
	}
}

// forgetChat drops every stream of chatID: the chat is being erased.
func (l *live) forgetChat(chatID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byChat, chatID)
}
