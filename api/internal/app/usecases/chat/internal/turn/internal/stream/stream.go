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

	// RunnerID is fixed at creation — the runner whose deltas started this
	// message. Never re-derived from "whichever runner is calling right
	// now": two runners can have messages open on the SAME chat at once (an
	// interrupted turn gracefully finishing while a new one has already
	// started after a provider switch), and only the creating runner may
	// ever close, forget, or claim this buffer.
	RunnerID string

	TurnID string

	chunks map[int]string

	highest int

	// nextArrival is the index an unsequenced increment gets: arrival order,
	// since the provider that sent it numbered none of its own chunks.
	nextArrival int

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
		RunnerID:     b.RunnerID,
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

	// waiters holds one channel per caller currently blocked in AwaitOpen,
	// keyed by chatID+"\x00"+runnerID, woken the instant that runner's next
	// message starts.
	waiters map[string][]chan struct{}
}

// New returns an empty set of per-chat message streams.
func New() *Streams {
	return &Streams{
		byChat:  make(map[string]map[string]*buffer),
		order:   make(map[string][]string),
		waiters: make(map[string][]chan struct{}),
	}
}

// Observe folds one streamed increment into its message and returns the frozen
// result. index is the increment's position, so out-of-order arrivals still
// assemble correctly and a gap leaves the message incomplete — but only when
// sequenced is true. A provider whose deltas carry no position of their own
// (see models.MessageDelta.Sequenced) reports every increment at index 0, and
// trusting that would fold every chunk onto the same slot; sequenced false
// instead assigns the next chunk arrival order, which is sound exactly because
// such a provider's own deltas arrive on one ordered stream to begin with. ok
// is false for an increment that names no chat or no message.
func (s *Streams) Observe(
	chatID string,
	runnerID string,
	turnID string,
	messageID string,
	index int,
	sequenced bool,
	final bool,
	text string,
	now time.Time,
) (Message, bool) {
	if chatID == "" || messageID == "" {
		return Message{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	buffer := s.bufferLocked(chatID, runnerID, turnID, messageID)
	if !sequenced {
		index = buffer.nextArrival
		buffer.nextArrival++
	}
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
	runnerID string,
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
	buffer := &buffer{ID: messageID, RunnerID: runnerID, TurnID: turnID, chunks: make(map[int]string)}
	messages[messageID] = buffer
	s.order[chatID] = append(s.order[chatID], messageID)
	s.evictLocked(chatID)
	s.wakeLocked(chatID, runnerID)
	return buffer
}

// wakeLocked releases every AwaitOpen caller parked on this runner's chat: a
// new message just started, so whatever they were waiting to learn is now
// answerable from Open.
func (s *Streams) wakeLocked(chatID, runnerID string) {
	key := chatID + "\x00" + runnerID
	for _, ch := range s.waiters[key] {
		close(ch)
	}
	delete(s.waiters, key)
}

// Open returns a frozen copy of every message THIS RUNNER still has open on
// the chat, oldest first. Filtered by runner — not just chat — so that
// closing one runner's turn can never scoop up, and misattribute, a
// DIFFERENT runner's still-open message. That is not a hypothetical: a turn
// that was asked to stop is a graceful request, not a kill (see
// runner/lifecycle.go), so an "interrupted" runner can still be mid-message
// here when a different runner (a provider switch) opens and closes its own
// turn — and an unfiltered Open would have handed that provider the
// interrupted runner's text as if it were its own.
func (s *Streams) Open(chatID, runnerID string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openLocked(chatID, runnerID)
}

// AwaitOpen is Open, except that when THIS RUNNER has nothing open yet it
// waits up to timeout for a message to start before giving up. It exists for
// the race between the hook that reports a turn's final text and the
// increments that assemble that SAME message: each canonical event is its own
// independently delivered hook, with no ordering guarantee against a
// different event's delivery, so "the turn-closing hook arrived first" and
// "nothing was ever streamed" are indistinguishable without briefly waiting
// for the alternative to resolve itself. Returns immediately, with no wait,
// whenever something is already open.
func (s *Streams) AwaitOpen(chatID, runnerID string, timeout time.Duration) []Message {
	s.mu.Lock()
	if open := s.openLocked(chatID, runnerID); len(open) > 0 {
		s.mu.Unlock()
		return open
	}
	key := chatID + "\x00" + runnerID
	woken := make(chan struct{})
	s.waiters[key] = append(s.waiters[key], woken)
	s.mu.Unlock()

	select {
	case <-woken:
	case <-time.After(timeout):
		s.mu.Lock()
		s.removeWaiterLocked(key, woken)
		s.mu.Unlock()
	}
	return s.Open(chatID, runnerID)
}

// removeWaiterLocked drops one timed-out waiter so a runner whose messages
// never stream (a hooks-only provider that reports everything in the
// terminating hook) does not accumulate one dead channel per turn forever.
func (s *Streams) removeWaiterLocked(key string, ch chan struct{}) {
	list := s.waiters[key]
	for i, c := range list {
		if c == ch {
			s.waiters[key] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(s.waiters[key]) == 0 {
		delete(s.waiters, key)
	}
}

func (s *Streams) openLocked(chatID, runnerID string) []Message {
	ids := s.order[chatID]
	out := make([]Message, 0, len(ids))
	for _, id := range ids {
		buf, ok := s.byChat[chatID][id]
		if !ok || buf.RunnerID != runnerID {
			continue
		}
		out = append(out, buf.snapshot())
	}
	return out
}

// Unfinished returns the open messages THIS RUNNER's provider never marked
// final — the ones its turn can end without ever completing.
func (s *Streams) Unfinished(chatID, runnerID string) []Message {
	out := make([]Message, 0, 2)
	for _, message := range s.Open(chatID, runnerID) {
		out = appendUnfinished(out, message)
	}
	return out
}

// UnfinishedAcrossRunners is Unfinished without the runner filter — for the
// one legitimate chat-wide question this package answers ("is ANYTHING for
// this chat still moving, regardless of which runner", used to decide
// whether a chat is quiet enough to sweep), never for attributing text to a
// provider. See UnfinishedSince, its only caller.
func (s *Streams) UnfinishedAcrossRunners(chatID string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.order[chatID]
	out := make([]Message, 0, 2)
	for _, id := range ids {
		buf, ok := s.byChat[chatID][id]
		if !ok {
			continue
		}
		out = appendUnfinished(out, buf.snapshot())
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

// IndexOf returns messageID's position in THIS RUNNER's arrival order within
// the chat — its ItemIndex within the turn, for giving several persisted
// messages from one turn (a Codex reply split across several items)
// distinct, orderable positions while they share one DisplayOrder (see
// domain.ActivityTurn). Filtered by runner for the same reason Open is: two
// runners' messages sharing one chat-wide arrival list must never be
// numbered as if they were one runner's turn. Zero for a messageID never
// observed here (a message recorded straight from the terminating hook,
// with no delta of its own — the first and only item either way).
func (s *Streams) IndexOf(chatID, runnerID, messageID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := 0
	for _, id := range s.order[chatID] {
		buf, ok := s.byChat[chatID][id]
		if !ok || buf.RunnerID != runnerID {
			continue
		}
		if id == messageID {
			return index
		}
		index++
	}
	return 0
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

// Forget drops every message THIS RUNNER has open on the chat. A turn that
// has been recorded owns its text now; keeping the buffers would re-record
// them. Scoped to the runner, not the whole chat, so closing one runner's
// turn can never discard a DIFFERENT runner's still-open message out from
// under it.
func (s *Streams) Forget(chatID, runnerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := s.byChat[chatID]
	ids := s.order[chatID]
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		buf, ok := messages[id]
		if !ok {
			continue
		}
		if buf.RunnerID == runnerID {
			delete(messages, id)
			continue
		}
		kept = append(kept, id)
	}
	if len(messages) == 0 {
		delete(s.byChat, chatID)
	}
	if len(kept) == 0 {
		delete(s.order, chatID)
	} else {
		s.order[chatID] = kept
	}
}

func (s *Streams) evictLocked(chatID string) {
	ids := s.order[chatID]
	for len(ids) > MaxOpenPerChat {
		delete(s.byChat[chatID], ids[0])
		ids = ids[1:]
	}
	s.order[chatID] = ids
}
