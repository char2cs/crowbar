// Package stream assembles each streamed assistant message from the increments
// its provider reports, because the hook that terminates a turn carries only the
// LAST message of that turn.
package stream

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxOpenPerChat bounds how many messages one chat may have open at once. A CLI
// that streams without ever terminating a message would otherwise grow the
// buffer for the life of the daemon; past the bound the oldest is evicted.
const MaxOpenPerChat = 64

type buffer struct {
	ID string

	TurnID string

	chunks map[int]string

	highest int

	Final bool

	LastAt time.Time

	recordedText string
}

func (b *buffer) Text() string {
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

func (b *buffer) Complete() bool {
	for i := 0; i <= b.highest; i++ {
		if _, ok := b.chunks[i]; !ok {
			return false
		}
	}
	return true
}

func (b *buffer) snapshot() Message {
	return Message{
		ID:           b.ID,
		TurnID:       b.TurnID,
		Text:         b.Text(),
		RecordedText: b.recordedText,
		Final:        b.Final,
		Complete:     b.Complete(),
		LastAt:       b.LastAt,
	}
}

// Streams holds every assistant message still being assembled, per chat. It is
// bounded: a chat keeps at most MaxOpenPerChat open messages and the oldest is
// evicted, so a CLI that streams without ever terminating cannot grow it without
// limit.
type Streams struct {
	mu     sync.Mutex
	byChat map[string]map[string]*buffer

	order map[string][]string
}

// New returns an empty set of per-chat message streams.
func New() *Streams {
	return &Streams{
		byChat: make(map[string]map[string]*buffer),
		order:  make(map[string][]string),
	}
}

// Observe folds one streamed increment into its message and returns the frozen
// result. index is the increment's position, so out-of-order arrivals still
// assemble correctly and a gap leaves the message incomplete. ok is false for an
// increment that names no chat or no message.
func (s *Streams) Observe(
	chatID string,
	turnID string,
	messageID string,
	index int,
	final bool,
	text string,
	now time.Time,
) (Message, bool) {
	if chatID == "" || messageID == "" {
		return Message{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	buffer := s.bufferLocked(chatID, turnID, messageID)
	buffer.chunks[index] = text
	if index > buffer.highest {
		buffer.highest = index
	}
	if final {
		buffer.Final = true
	}
	buffer.LastAt = now
	return buffer.snapshot(), true
}

func (s *Streams) bufferLocked(
	chatID string,
	turnID string,
	messageID string,
) *buffer {
	messages, ok := s.byChat[chatID]
	if !ok {
		messages = make(map[string]*buffer)
		s.byChat[chatID] = messages
	}
	if buffer, ok := messages[messageID]; ok {
		return buffer
	}
	buffer := &buffer{ID: messageID, TurnID: turnID, chunks: make(map[int]string)}
	messages[messageID] = buffer
	s.order[chatID] = append(s.order[chatID], messageID)
	s.evictLocked(chatID)
	return buffer
}

// Open returns a frozen copy of every message still open on the chat, oldest
// first.
func (s *Streams) Open(
	chatID string,
) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.order[chatID]
	out := make([]Message, 0, len(ids))
	for _, id := range ids {
		out = s.appendSnapshotLocked(out, chatID, id)
	}
	return out
}

func (s *Streams) appendSnapshotLocked(
	out []Message,
	chatID string,
	messageID string,
) []Message {
	buffer, ok := s.byChat[chatID][messageID]
	if !ok {
		return out
	}
	return append(out, buffer.snapshot())
}

// Unfinished returns the open messages whose provider never marked them final —
// the ones a turn can end without ever completing.
func (s *Streams) Unfinished(
	chatID string,
) []Message {
	out := make([]Message, 0, 2)
	for _, message := range s.Open(chatID) {
		out = appendUnfinished(out, message)
	}
	return out
}

func appendUnfinished(
	out []Message,
	message Message,
) []Message {
	if message.Final {
		return out
	}
	return append(out, message)
}

// MarkRecorded records the exact text that was persisted for a message, so a
// later append only writes the tail that grew after it.
func (s *Streams) MarkRecorded(chatID, messageID, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if buffer, ok := s.byChat[chatID][messageID]; ok {
		buffer.recordedText = text
	}
}

// Forget drops every message open on the chat. A turn that has been recorded owns
// its text now; keeping the buffers would re-record them.
func (s *Streams) Forget(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byChat, chatID)
	delete(s.order, chatID)
}

func (s *Streams) evictLocked(chatID string) {
	ids := s.order[chatID]
	for len(ids) > MaxOpenPerChat {
		delete(s.byChat[chatID], ids[0])
		ids = ids[1:]
	}
	s.order[chatID] = ids
}
