package ingress

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const (
	KindAnswer     = ""
	KindReasoning  = "reasoning"
	KindToolOutput = "tool_output"
	KindDiff       = "diff"
)

const StreamAnswer = "answer"

const toolOutputWindow = 64 << 10

type LiveDelta struct {
	TurnID, Scope, ID, Kind string
	Index                   int
	Numbered                bool
	Replace                 bool
	Text                    string
}

func (r *Recorder) unrecorded(chatID, runnerID string) Unrecorded {
	return Unrecorded{
		Answers:   r.live.unrecorded(chatID, runnerID, "", KindAnswer),
		Reasoning: r.live.unrecorded(chatID, runnerID, "", KindReasoning),
		Diff:      r.live.unrecorded(chatID, runnerID, "", KindDiff),
	}
}

func (r *Recorder) publishLive(chat domain.Chat, runnerID string, d LiveDelta) {
	if fed := r.live.observe(chat.ID, runnerID, d); fed != nil {
		r.frames.FeedDelta(chat.ID, *fed)
	}
}

func (r *Recorder) Snapshot(chatID string, st ledger.State) []ledger.FeedDelta {
	return r.live.snapshot(chatID, st)
}

func (r *Recorder) ExitRows(st ledger.State, chatID, runnerID string) ([]ledger.Row, error) {
	return Salvage(st, r.live.unrecorded(chatID, runnerID, "", KindAnswer))
}

func (r *Recorder) LiveDiff(chatID, runnerID string) string {
	return lastText(r.live.unrecorded(chatID, runnerID, "", KindDiff), "")
}

func (r *Recorder) Retire(chatID, runnerID string) {
	r.live.forget(chatID, runnerID, "")
	r.settleUnclaimed(context.Background(), chatID, r.unbound.releaseRunner(chatID, runnerID))
}

type liveKey struct {
	runnerID, turnID, scope, id, kind string
}

type liveBlock struct {
	parts    map[int]string
	next     int
	numbered bool
	order    int
	dropped  int
	n        int
}

type live struct {
	mu     sync.Mutex
	byChat map[string]map[liveKey]*liveBlock
	began  int
}

func newLive() *live { return &live{byChat: map[string]map[liveKey]*liveBlock{}} }

func (l *live) observe(chatID, runnerID string, d LiveDelta) *ledger.FeedDelta {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := liveKey{runnerID: runnerID, turnID: d.TurnID, scope: d.Scope, id: d.ID, kind: d.Kind}
	b := l.blockLocked(chatID, k)
	before := b.join(d.Kind)
	b.fold(d)
	after := b.join(d.Kind)
	if d.Kind == KindToolOutput {
		b.trim()
	}
	if b.numbered || after == before {
		return nil
	}
	b.n++
	fed := k.feed(b.n, b.text(d.Kind), true)
	if strings.HasPrefix(after, before) {
		fed.Text, fed.Replace = after[len(before):], false
	}
	return &fed
}

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

func (k liveKey) feed(n int, text string, replace bool) ledger.FeedDelta {
	stream := k.kind
	if stream == KindAnswer {
		stream = StreamAnswer
	}
	return ledger.FeedDelta{ID: k.id, TurnID: k.turnID, Scope: k.scope, Stream: stream, N: n, Text: text, Replace: replace}
}

func (l *live) snapshot(chatID string, st ledger.State) []ledger.FeedDelta {
	l.mu.Lock()
	defer l.mu.Unlock()
	blocks := l.byChat[chatID]
	keys := slices.Collect(maps.Keys(blocks))
	slices.SortFunc(keys, func(a, b liveKey) int { return blocks[a].order - blocks[b].order })
	var out []ledger.FeedDelta
	for _, k := range keys {
		if b := blocks[k]; !b.numbered && b.n > 0 && !k.recorded(st) {
			out = append(out, k.feed(b.n, b.text(k.kind), true))
		}
	}
	return out
}

func (k liveKey) recorded(st ledger.State) bool {
	switch k.kind {
	case KindAnswer:
		return st.HasChunk(k.scope, k.id, ledger.IndexComplete)
	case KindToolOutput:
		return st.HasToolPhase(k.scope, k.id, ledger.ToolPost) || st.HasToolPhase(k.scope, k.id, ledger.ToolFail)
	}
	return false
}

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

func (l *live) blockLocked(chatID string, k liveKey) *liveBlock {
	blocks := l.byChat[chatID]
	if blocks == nil {
		blocks = map[liveKey]*liveBlock{}
		l.byChat[chatID] = blocks
	}
	b := blocks[k]
	if b == nil {
		l.began++
		b = &liveBlock{parts: map[int]string{}, order: l.began}
		blocks[k] = b
	}
	return b
}

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

type liveText struct {
	TurnID, Scope, ID, Text string
}

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

func (l *live) forgetChat(chatID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byChat, chatID)
}
