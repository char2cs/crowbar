package turn

import (
	"sort"
	"strings"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// DeltaKindReasoning tags a streamed text block as the model thinking rather than
// answering. It travels on the same live channel as the answer and is the only
// thing that tells a client to render it as a thought.
const DeltaKindReasoning = "reasoning"

// reasoningBuffer accumulates the parts of one thinking block so the live channel
// can carry the text SO FAR, exactly as the answer channel does — a dropped frame
// then costs nothing, where an increment-only channel would leave a permanent hole.
//
// It is deliberately in memory and deliberately never written to the ledger. A
// thought is a view of a turn in progress, not a record of it: keeping every
// reasoning token of every turn would multiply the transcript's size for text
// nobody scrolls back to, and it would have to be reconciled with the answer on
// every resume. It is dropped when the turn that produced it ends.
//
// Parts are ordered by their provider-reported index, not by arrival. codex emits
// a thinking block as numbered summary parts (summaryIndex), and a reasoning model
// interleaves them with tool work, so arrival order is not text order.
type reasoningBuffer struct {
	mu     sync.Mutex
	byChat map[string]map[string]*reasoningBlock
}

type reasoningBlock struct {
	parts map[int]*strings.Builder
}

func newReasoningBuffer() *reasoningBuffer {
	return &reasoningBuffer{byChat: make(map[string]map[string]*reasoningBlock)}
}

// observe appends one delta and returns the whole block as it now reads.
func (b *reasoningBuffer) observe(chatID, blockID string, index int, text string) string {
	if b == nil {
		return text
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	blocks, ok := b.byChat[chatID]
	if !ok {
		blocks = make(map[string]*reasoningBlock)
		b.byChat[chatID] = blocks
	}
	block, ok := blocks[blockID]
	if !ok {
		block = &reasoningBlock{parts: make(map[int]*strings.Builder)}
		blocks[blockID] = block
	}
	part, ok := block.parts[index]
	if !ok {
		part = &strings.Builder{}
		block.parts[index] = part
	}
	part.WriteString(text)

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

// forget drops every thinking block held for a chat. Called when a turn ends: the
// thought belonged to that turn and the answer has superseded it.
func (b *reasoningBuffer) forget(chatID string) {
	// Nil-safe: a Turns built field-by-field in a test has no buffer, and the
	// close paths must not care.
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.byChat, chatID)
}

// recordReasoningDelta publishes the model's thinking as it arrives.
//
// It writes NOTHING durable — no ledger turn, no activity record, no assistant
// message. That is the whole difference between this and recordMessageDelta, and
// it is why a provider mapping reasoning_delta cannot corrupt a transcript.
func (t *Turns) recordReasoningDelta(
	chat domain.Chat,
	ev engineagents.CanonicalEvent,
) {
	if ev.Delta == nil || ev.Delta.Text == "" {
		return
	}
	if t.messageDelta == nil {
		return // nobody is listening; a thought nobody sees is not worth buffering
	}
	// A provider that streams one undifferentiated thought stream per turn maps no
	// message_id (the vocabulary makes it optional for exactly that); the turn is
	// grouping enough, and an empty key groups them together.
	blockID := ev.Delta.MessageID
	if blockID == "" {
		blockID = ev.Delta.TurnID
	}
	text := t.reasoning.observe(chat.ID, blockID, ev.Delta.Index, ev.Delta.Text)
	t.messageDelta(chat.ID, chat.WorkspaceID, blockID, text, DeltaKindReasoning)
}

// ForgetReasoning drops the thinking held for a chat. Exported for the turn-close
// paths in this package's siblings.
func (t *Turns) ForgetReasoning(chatID string) { t.reasoning.forget(chatID) }
