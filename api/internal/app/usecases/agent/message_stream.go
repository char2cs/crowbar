package agent

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const maxOpenMessagesPerChat = 64

type messageBuffer struct {
	ID string

	TurnID string

	chunks map[int]string

	highest int

	Final bool

	LastAt time.Time

	recordedText string
}

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

func (b *messageBuffer) Complete() bool {
	for i := 0; i <= b.highest; i++ {
		if _, ok := b.chunks[i]; !ok {
			return false
		}
	}
	return true
}

func (b *messageBuffer) snapshot() Message {
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

type messageStreams struct {
	mu     sync.Mutex
	byChat map[string]map[string]*messageBuffer

	order map[string][]string
}

func newMessageStreams() *messageStreams {
	return &messageStreams{
		byChat: make(map[string]map[string]*messageBuffer),
		order:  make(map[string][]string),
	}
}

func (s *messageStreams) observe(
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

func (s *messageStreams) bufferLocked(
	chatID string,
	turnID string,
	messageID string,
) *messageBuffer {
	messages, ok := s.byChat[chatID]
	if !ok {
		messages = make(map[string]*messageBuffer)
		s.byChat[chatID] = messages
	}
	if buffer, ok := messages[messageID]; ok {
		return buffer
	}
	buffer := &messageBuffer{ID: messageID, TurnID: turnID, chunks: make(map[int]string)}
	messages[messageID] = buffer
	s.order[chatID] = append(s.order[chatID], messageID)
	s.evictLocked(chatID)
	return buffer
}

func (s *messageStreams) openMessages(
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

func (s *messageStreams) appendSnapshotLocked(
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

func (s *messageStreams) unfinished(
	chatID string,
) []Message {
	out := make([]Message, 0, 2)
	for _, message := range s.openMessages(chatID) {
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

func (s *messageStreams) markRecorded(chatID, messageID, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if buffer, ok := s.byChat[chatID][messageID]; ok {
		buffer.recordedText = text
	}
}

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
