package turn

import (
	"sort"
	"strings"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The kinds of streamed text that are NOT the agent's answer. The answer's own
// kind is the empty string — the stream that existed before there was more than
// one, and the only one the ledger ever records.
const (
	// DeltaKindReasoning is the model thinking out loud.
	DeltaKindReasoning = "reasoning"
	// DeltaKindToolOutput is a running tool's output as it is produced — the
	// lines of a build, a test run, a long-running command.
	DeltaKindToolOutput = "tool_output"
)

// liveText accumulates streamed text that is shown while it happens and never
// recorded, so the live channel can carry the text SO FAR exactly as the answer
// channel does — a dropped frame then costs nothing, where an increment-only
// channel would leave a permanent hole.
//
// It is deliberately in memory and deliberately never written to the ledger.
// A thought, or the scrolling output of a command, is a view of a turn in
// progress rather than a record of it: keeping every token of it would multiply
// the transcript's size for text nobody scrolls back to, and it would have to be
// reconciled with the answer on every resume. It is dropped when the turn that
// produced it ends.
//
// Keyed by chat → kind → block. The kind is part of the key so a tool's output
// and the reasoning running alongside it can never overwrite one another, and
// parts are ordered by their provider-reported index rather than by arrival:
// codex emits a thinking block as numbered summary parts and interleaves them
// with tool work, so arrival order is not text order.
type liveText struct {
	mu     sync.Mutex
	byChat map[string]map[string]map[string]*textBlock
}

type textBlock struct {
	parts map[int]*strings.Builder
}

func newLiveText() *liveText {
	return &liveText{byChat: make(map[string]map[string]map[string]*textBlock)}
}

// observe appends one delta and returns the whole block as it now reads.
func (b *liveText) observe(chatID, kind, blockID string, index int, text string) string {
	if b == nil {
		return text
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	kinds, ok := b.byChat[chatID]
	if !ok {
		kinds = make(map[string]map[string]*textBlock)
		b.byChat[chatID] = kinds
	}
	blocks, ok := kinds[kind]
	if !ok {
		blocks = make(map[string]*textBlock)
		kinds[kind] = blocks
	}
	block, ok := blocks[blockID]
	if !ok {
		block = &textBlock{parts: make(map[int]*strings.Builder)}
		blocks[blockID] = block
	}
	part, ok := block.parts[index]
	if !ok {
		part = &strings.Builder{}
		block.parts[index] = part
	}
	part.WriteString(text)

	if len(block.parts) == 1 {
		return part.String()
	}
	order := make([]int, 0, len(block.parts))
	for i := range block.parts {
		order = append(order, i)
	}
	sort.Ints(order)
	var out strings.Builder
	for n, i := range order {
		if n > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(block.parts[i].String())
	}
	return out.String()
}

// forget drops every live block held for a chat. Called when a turn ends: the
// text belonged to that turn and the answer has superseded it.
//
// Nil-safe: a Turns built field-by-field in a test has no buffer, and the close
// paths must not care.
func (b *liveText) forget(chatID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.byChat, chatID)
}

// recordLiveText publishes one non-answer text stream as it arrives.
//
// It writes NOTHING durable — no ledger turn, no activity record, no assistant
// message. That is the whole difference between this and recordMessageDelta, and
// it is why a provider mapping one of these kinds cannot corrupt a transcript.
func (t *Turns) recordLiveText(
	chat domain.Chat,
	ev engineagents.CanonicalEvent,
	kind string,
) {
	if ev.Delta == nil || ev.Delta.Text == "" {
		return
	}
	if t.messageDelta == nil {
		return // nobody is listening; text nobody sees is not worth buffering
	}
	// A provider that streams one undifferentiated stream per turn maps no
	// message_id (the vocabulary makes it optional for exactly that); the turn is
	// grouping enough, and an empty key groups them together.
	blockID := ev.Delta.MessageID
	if blockID == "" {
		blockID = ev.Delta.TurnID
	}
	text := t.live.observe(chat.ID, kind, blockID, ev.Delta.Index, ev.Delta.Text)
	t.messageDelta(chat.ID, chat.WorkspaceID, blockID, text, kind)
}
