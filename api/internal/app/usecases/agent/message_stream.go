package agent

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Assembling what the agent said from the increments it said it in.
//
// THE DEFECT THIS CLOSES. A provider's terminating hook reports ONE message — the
// LAST of the turn. An agent that speaks, reaches for a tool, and speaks again
// produced two messages and reported one, so everything before the final tool call
// was never recorded at all. Measured against claude 2.1.236: a turn of
// ALPHA -> Bash -> OMEGA left `last_assistant_message` carrying OMEGA alone, and
// 260 characters of real assistant prose went nowhere.
//
// WHERE THE MISSING TEXT COMES FROM NOW. The provider's own streaming hook, which
// carries each message in increments while it is being produced, keyed by a
// message id and ordered by a contiguous index. This replaces reading the
// provider's transcript file, which is a private implementation detail Crowbar has
// no business opening: a file format nobody publishes is not a contract, and it is
// not something to ship to users. Everything here comes off a declared hook.
//
// WHAT MAKES IT SAFE:
//
//   - A provider that declares no streaming hook reaches none of this, and records
//     exactly what it recorded before: one message per turn, from the terminating
//     hook. codex is that provider today — it declares eleven events and none of
//     them streams.
//   - The terminating hook is a free RECONCILIATION PASS over the last message of
//     every turn, because it carries that message in full. Concatenated deltas were
//     measured byte-identical to it, so a mismatch means increments were lost and
//     the hook's copy wins.
//   - A gap in the index is a DETECTED hole rather than a silent one. Hook delivery
//     has no acknowledgement, so a chunk that never arrives would otherwise be a
//     message quietly recorded short.

// maxOpenMessagesPerChat bounds how many messages one chat may have in flight.
//
// Real turns hold one or two. The cap exists so a provider that never sends a
// terminating increment cannot grow this without limit; evicting the oldest costs
// at worst one message recorded from the terminating hook instead of from its
// increments, which is the same degradation a provider with no streaming hook
// lives with permanently.
const maxOpenMessagesPerChat = 64

// messageBuffer is one assistant message being assembled.
type messageBuffer struct {
	// ID is the provider's own message identity. Two messages of one turn differ
	// here and nowhere else — grouping by turn alone would concatenate them into
	// one thing the agent never said.
	ID string
	// TurnID groups the messages of a single turn.
	TurnID string

	// chunks is index -> text. A map rather than a slice because increments are
	// not guaranteed to arrive in order, and because a MISSING index has to stay
	// missing: an append-ordered slice would silently close the gap.
	chunks map[int]string
	// highest is the largest index seen, so a hole can be told from an end.
	highest int

	// Final marks that the provider said this message is complete.
	//
	// It is NOT guaranteed to arrive: measured on claude 2.1.236, a human interrupt
	// stops the increments and fires no hook of any kind. An unfinished message is
	// therefore evidence in its own right — see the abandoned-message sweep.
	Final bool

	// LastAt is when the most recent increment arrived, and it is the whole of the
	// interrupt clock: an unfinished message whose increments have stopped is a
	// message that was cut off.
	LastAt time.Time

	// recordedText is what was last written to the ledger for this message, so a
	// re-record with nothing new to say can be skipped.
	recordedText string
}

// Text assembles the message from its increments, in index order.
//
// Increments are non-overlapping additions, never cumulative snapshots, so this
// is a concatenation and not a pick-the-longest. Measured byte-identical to the
// provider's own report of the same message.
func (b *messageBuffer) Text() string {
	if len(b.chunks) == 0 {
		return ""
	}
	indices := make([]int, 0, len(b.chunks))
	for i := range b.chunks {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	var sb strings.Builder
	for _, i := range indices {
		sb.WriteString(b.chunks[i])
	}
	return sb.String()
}

// Complete reports whether every increment from zero to the highest seen is
// present.
//
// This is the integrity check, and the only one available: hook delivery is
// unacknowledged, so a dropped increment is otherwise indistinguishable from a
// message that was simply shorter. An incomplete message is still recorded — a
// partial answer is better than none — but the caller knows not to trust it as
// the provider's final word, and the terminating hook's copy supersedes it.
func (b *messageBuffer) Complete() bool {
	for i := 0; i <= b.highest; i++ {
		if _, ok := b.chunks[i]; !ok {
			return false
		}
	}
	return true
}

// messageStreams is the per-process registry of messages being assembled, keyed by
// chat and then by the provider's message id.
//
// In memory rather than durable, deliberately. An increment is worth having while
// the turn is live and worthless afterwards: the ledger holds the message, and a
// daemon restart mid-turn loses at most the increments of the message in flight,
// which the terminating hook then supplies in full anyway.
type messageStreams struct {
	mu     sync.Mutex
	byChat map[string]map[string]*messageBuffer
	// order preserves arrival order per chat, so eviction takes the oldest and a
	// turn's messages are recorded in the sequence the agent said them.
	order map[string][]string
}

func newMessageStreams() *messageStreams {
	return &messageStreams{
		byChat: make(map[string]map[string]*messageBuffer),
		order:  make(map[string][]string),
	}
}

// observe folds one increment in and returns the message it belongs to.
//
// An increment with no message id is dropped: it cannot be grouped, and appending
// it to whatever came last would attribute text to a message that did not contain
// it. The descriptor rules refuse a mapping that omits the id, so this is a
// provider that stopped sending one rather than a descriptor that never asked.
func (s *messageStreams) observe(
	chatID, turnID, messageID string,
	index int,
	final bool,
	text string,
	now time.Time,
) (*messageBuffer, bool) {
	if chatID == "" || messageID == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	messages, ok := s.byChat[chatID]
	if !ok {
		messages = make(map[string]*messageBuffer)
		s.byChat[chatID] = messages
	}
	buffer, ok := messages[messageID]
	if !ok {
		buffer = &messageBuffer{ID: messageID, TurnID: turnID, chunks: make(map[int]string)}
		messages[messageID] = buffer
		s.order[chatID] = append(s.order[chatID], messageID)
		s.evictLocked(chatID)
	}
	// A redelivered increment must not be appended twice. Hook delivery is
	// at-least-once by construction — the relay retries — and the index is what
	// makes a repeat identifiable as one.
	buffer.chunks[index] = text
	if index > buffer.highest {
		buffer.highest = index
	}
	if final {
		buffer.Final = true
	}
	buffer.LastAt = now
	return buffer, true
}

// openMessages returns the chat's messages in arrival order.
func (s *messageStreams) openMessages(chatID string) []*messageBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.order[chatID]
	out := make([]*messageBuffer, 0, len(ids))
	for _, id := range ids {
		if buffer, ok := s.byChat[chatID][id]; ok {
			out = append(out, buffer)
		}
	}
	return out
}

// unfinished returns the chat's messages the provider never marked complete,
// oldest first, along with when each last grew.
//
// This is what the interrupt sweep reads. A finished message is not evidence of
// anything: it ended the way it was supposed to.
func (s *messageStreams) unfinished(chatID string) []*messageBuffer {
	out := make([]*messageBuffer, 0, 2)
	for _, buffer := range s.openMessages(chatID) {
		if !buffer.Final {
			out = append(out, buffer)
		}
	}
	return out
}

// markRecorded remembers what was written for a message, so an unchanged message
// is not rewritten on every increment.
func (s *messageStreams) markRecorded(chatID, messageID, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if buffer, ok := s.byChat[chatID][messageID]; ok {
		buffer.recordedText = text
	}
}

// forget drops a chat's in-flight messages. Called when a turn ends, because the
// ledger now holds them and an increment arriving afterwards belongs to the next
// turn's messages, not to these.
func (s *messageStreams) forget(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byChat, chatID)
	delete(s.order, chatID)
}

func (s *messageStreams) evictLocked(chatID string) {
	ids := s.order[chatID]
	for len(ids) > maxOpenMessagesPerChat {
		delete(s.byChat[chatID], ids[0])
		ids = ids[1:]
	}
	s.order[chatID] = ids
}
