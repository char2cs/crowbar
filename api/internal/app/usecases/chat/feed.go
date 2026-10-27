package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// The frame types a chat feed sends. A feed ends after a resync or a deleted.
const (
	feedRow     = "row"
	feedReady   = "ready"
	feedDelta   = "delta"
	FeedResync  = "resync"
	FeedDeleted = "deleted"
)

// feedCapacity is how many frames a feed's client may leave unread before it
// is told to resync.
const feedCapacity = 1024

// feedReplayPage is how many rows one read of a replay takes.
const feedReplayPage = 512

// FeedFrame is one frame on a chat feed. Row is set on "row", Delta on "delta",
// Epoch and Seq on "ready", Seq on "resync".
type FeedFrame struct {
	Type  string      `json:"type"` // "row" | "ready" | "delta" | "resync" | "deleted"
	Row   *ledger.Row `json:"row,omitempty"`
	Epoch string      `json:"epoch,omitempty"`
	Seq   int64       `json:"seq,omitempty"`
	Delta *FeedDelta  `json:"delta,omitempty"`
}

// FeedDelta is one frame of streamed text no row holds.
type FeedDelta = ingress.FeedDelta

func rowFrame(r ledger.Row) FeedFrame { return FeedFrame{Type: feedRow, Row: &r} }

// errFeedEnded is an attach that sent the frame its feed ends on.
var errFeedEnded = errors.New("chat feed: ended while attaching")

// OpenFeed is chatID's feed: rows after since, ready, a snapshot of its live
// streams, then live frames. It registers before reading the head, so a row
// committed meanwhile is sent once; an unknown chat is apperr.ErrNotFound.
func (u *Usecase) OpenFeed(ctx context.Context, chatID string, since int64) (<-chan FeedFrame, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			return nil, fmt.Errorf("chat feed: %s: %w: %w", chatID, err, apperr.ErrNotFound)
		}
		return nil, fmt.Errorf("chat feed: %s: %w", chatID, err)
	}
	return u.feeds.open(ctx, chatID, since, func(emit func(FeedFrame) bool) error {
		return u.attachFeed(ctx, chatID, since, emit)
	}), nil
}

// attachFeed replays chatID's rows after since up to its head, then sends the
// ready frame and the snapshot of what no row up to the head holds. A purge
// that ran before the feed registered is told as deleted.
func (u *Usecase) attachFeed(ctx context.Context, chatID string, since int64, emit func(FeedFrame) bool) error {
	if _, err := u.chats.GetChat(ctx, chatID); errors.Is(err, agentchat.ErrNotFound) {
		emit(FeedFrame{Type: FeedDeleted})
		return errFeedEnded
	} else if err != nil {
		return fmt.Errorf("chat feed: %s: %w", chatID, err)
	}
	st, err := u.ledger.State(ctx, chatID)
	if err != nil {
		return fmt.Errorf("chat feed: %s: head: %w", chatID, err)
	}
	head := st.Head()
	if since > head {
		// A since this ledger never reached: the client rebuilds from the head.
		emit(FeedFrame{Type: FeedResync, Seq: head})
		return errFeedEnded
	}
	for after := since; after < head; {
		rows, err := u.ledger.Read(ctx, chatID, after, int(min(head-after, feedReplayPage)))
		if err != nil {
			return fmt.Errorf("chat feed: %s: replay: %w", chatID, err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if !emit(rowFrame(r)) {
				return ctx.Err()
			}
		}
		after = rows[len(rows)-1].Seq
	}
	if !emit(FeedFrame{Type: feedReady, Epoch: u.epoch, Seq: head}) {
		return ctx.Err()
	}
	for _, d := range u.recorder.Snapshot(chatID, st) {
		if !emit(FeedFrame{Type: feedDelta, Delta: &d}) {
			return ctx.Err()
		}
	}
	return nil
}

// feeds is every open chat feed. Publishing never blocks: each feed queues
// what it is sent, and one goroutine per feed writes it out.
type feeds struct {
	capacity int
	mu       sync.Mutex
	byChat   map[string]map[*feedSub]struct{}
}

func newFeeds(capacity int) *feeds {
	return &feeds{capacity: capacity, byChat: map[string]map[*feedSub]struct{}{}}
}

// feedSub is one feed's queue. Unread counts the frame its writer holds, so a
// client is never more than capacity frames behind without being told.
type feedSub struct {
	wake    chan struct{}
	mu      sync.Mutex
	queue   []FeedFrame
	holding bool
	end     string // the frame it ends on once the queue before it is sent
}

// open registers a feed of chatID, then runs attach on its own goroutine: what
// attach emits comes first, then what was published meanwhile, without the
// rows the client was already sent (since counts as sent).
func (f *feeds) open(
	ctx context.Context, chatID string, since int64, attach func(emit func(FeedFrame) bool) error,
) <-chan FeedFrame {
	s := &feedSub{wake: make(chan struct{}, 1)}
	f.mu.Lock()
	if f.byChat[chatID] == nil {
		f.byChat[chatID] = map[*feedSub]struct{}{}
	}
	f.byChat[chatID][s] = struct{}{}
	f.mu.Unlock()
	out := make(chan FeedFrame)
	safego.Go("chat.feed", func() {
		defer close(out)
		defer f.leave(chatID, s)
		s.pump(ctx, chatID, out, since, attach)
	})
	return out
}

func (f *feeds) leave(chatID string, s *feedSub) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byChat[chatID], s)
	if len(f.byChat[chatID]) == 0 {
		delete(f.byChat, chatID)
	}
}

func (f *feeds) subs(chatID string) []*feedSub {
	f.mu.Lock()
	defer f.mu.Unlock()
	subs := make([]*feedSub, 0, len(f.byChat[chatID]))
	for s := range f.byChat[chatID] {
		subs = append(subs, s)
	}
	return subs
}

// rows sends chatID's committed rows to its feeds. It is a ledger subscriber.
func (f *feeds) rows(chatID string, rows []ledger.Row) {
	for _, s := range f.subs(chatID) {
		for _, r := range rows {
			s.offer(rowFrame(r), f.capacity)
		}
	}
}

// delta sends a frame of streamed text to chatID's feeds.
func (f *feeds) delta(chatID string, d FeedDelta) {
	for _, s := range f.subs(chatID) {
		s.offer(FeedFrame{Type: feedDelta, Delta: &d}, f.capacity)
	}
}

// deleted ends chatID's feeds: the chat is erased.
func (f *feeds) deleted(chatID string) {
	for _, s := range f.subs(chatID) {
		s.finish(FeedDeleted)
	}
}

func (s *feedSub) offer(fr FeedFrame, capacity int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.end != "" {
		return
	}
	unread := len(s.queue)
	if s.holding {
		unread++
	}
	if unread >= capacity {
		s.end = FeedResync
	} else {
		s.queue = append(s.queue, fr)
	}
	s.notify()
}

func (s *feedSub) finish(end string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.end == "" {
		s.end = end
	}
	s.notify()
}

func (s *feedSub) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// next takes the next queued frame, or reports the frame the feed ends on
// once nothing is queued ahead of it ("" while it goes on).
func (s *feedSub) next() (FeedFrame, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return FeedFrame{}, s.end, false
	}
	fr := s.queue[0]
	s.queue, s.holding = s.queue[1:], true
	return fr, "", true
}

func (s *feedSub) sent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.holding = false
}

// pump writes the feed out: attach's frames, then the queue, then its end.
func (s *feedSub) pump(
	ctx context.Context, chatID string, out chan<- FeedFrame, since int64, attach func(emit func(FeedFrame) bool) error,
) {
	w := &feedWriter{ctx: ctx, out: out, last: since}
	if err := attach(w.emit); err != nil {
		if !errors.Is(err, errFeedEnded) && ctx.Err() == nil {
			slog.Warn("chat feed: attach", "chat_id", chatID, "err", err)
			w.emit(FeedFrame{Type: FeedResync, Seq: w.last})
		}
		return
	}
	for s.step(w) {
	}
}

// step writes the queue's next frame, waiting for one; false once the feed is
// over. A row the client was already sent is skipped.
func (s *feedSub) step(w *feedWriter) bool {
	fr, end, ok := s.next()
	switch {
	case ok:
		sent := fr.Row != nil && fr.Row.Seq <= w.last || w.emit(fr)
		s.sent()
		return sent
	case end == FeedResync:
		w.emit(FeedFrame{Type: FeedResync, Seq: w.last})
		return false
	case end != "":
		w.emit(FeedFrame{Type: end})
		return false
	}
	select {
	case <-s.wake:
		return true
	case <-w.ctx.Done():
		return false
	}
}

// feedWriter sends a feed's frames, remembering the last row it sent.
type feedWriter struct {
	ctx  context.Context
	out  chan<- FeedFrame
	last int64
}

func (w *feedWriter) emit(fr FeedFrame) bool {
	select {
	case w.out <- fr:
		if fr.Row != nil {
			w.last = fr.Row.Seq
		}
		return true
	case <-w.ctx.Done():
		return false
	}
}
