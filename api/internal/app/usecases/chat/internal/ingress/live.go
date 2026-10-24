package ingress

import (
	"slices"
	"strings"
	"sync"
	"time"

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
	text, startedAt := r.live.observe(chat.ID, runnerID, d, time.Now())
	if d.Kind != KindAnswer {
		startedAt = time.Time{}
	}
	if d.Floor != "" && !(len(text) > len(d.Floor) && strings.HasPrefix(text, d.Floor)) {
		text = d.Floor
	}
	r.frames.MessageDelta(chat.ID, chat.WorkspaceID, d.ShownAs, text, d.Kind, startedAt)
}

// ExitRows are what runnerID's exit records of the answers it streamed with no
// row of their own; the caller commits them with the exit, then calls Retire.
func (r *Recorder) ExitRows(st ledger.State, chatID, runnerID string) ([]ledger.Row, error) {
	return Salvage(st, r.live.unrecorded(chatID, runnerID, "", KindAnswer))
}

// Retire drops runnerID's streamed text on chatID: its exit is recorded.
func (r *Recorder) Retire(chatID, runnerID string) { r.live.forget(chatID, runnerID, "") }

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

// observe folds d, from runnerID, into its stream and returns the stream's
// whole text and when it began. An answer's numbered increment takes its
// index's place, an unnumbered one the next; any other kind's increments
// append to their index's part.
func (l *live) observe(chatID, runnerID string, d LiveDelta, now time.Time) (string, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := liveKey{runnerID: runnerID, turnID: d.TurnID, scope: d.Scope, id: d.ID, kind: d.Kind}
	b := l.blockLocked(chatID, k, now)
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
	return b.text(d.Kind), b.at
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

func (b *liveBlock) text(kind string) string {
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
