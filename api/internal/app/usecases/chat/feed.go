package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

const (
	feedRow     = "row"
	FeedReady   = "ready"
	feedDelta   = "delta"
	FeedResync  = "resync"
	FeedDeleted = "deleted"
	FeedFailed  = "failed"
)

const feedCapacity = 1024

const feedReplayPage = 512

type FeedFrame struct {
	Type  string            `json:"type"`
	Row   *ledger.Row       `json:"row,omitempty"`
	Seq   int64             `json:"seq,omitempty"`
	From  int64             `json:"from,omitempty"`
	Delta *ledger.FeedDelta `json:"delta,omitempty"`
}

func rowFrame(r ledger.Row) FeedFrame { return FeedFrame{Type: feedRow, Row: &r} }

var errFeedEnded = errors.New("chat feed: ended while attaching")

func (u *Usecase) OpenFeed(ctx context.Context, chatID string, since, windowBytes int64) (<-chan FeedFrame, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			return nil, fmt.Errorf("chat feed: %s: %w: %w", chatID, err, problem.ErrNotFound)
		}
		return nil, fmt.Errorf("chat feed: %s: %w", chatID, err)
	}
	return u.feeds.open(ctx, chatID, since, func(emit func(FeedFrame) bool) error {
		return u.attachFeed(ctx, chatID, since, windowBytes, emit)
	}), nil
}

func (u *Usecase) ReadBlob(ctx context.Context, chatID, ref string) ([]byte, error) {
	if !ledgerstore.ValidRef(ref) {
		return nil, fmt.Errorf("chat blob: %q: %w", ref, problem.ErrInvalidArgument)
	}
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			return nil, fmt.Errorf("chat blob: %s: %w: %w", chatID, err, problem.ErrNotFound)
		}
		return nil, fmt.Errorf("chat blob: %s: %w", chatID, err)
	}
	data, err := u.blobs.Get(ref)
	if errors.Is(err, ledgerstore.ErrBlobNotFound) {
		return nil, fmt.Errorf("chat blob: %s: %w: %w", ref, err, problem.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("chat blob: %s: %w", ref, err)
	}
	return data, nil
}

func (u *Usecase) attachFeed(ctx context.Context, chatID string, since, windowBytes int64, emit func(FeedFrame) bool) error {
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
		emit(FeedFrame{Type: FeedResync, Seq: head})
		return errFeedEnded
	}
	from := int64(0)
	if since < 0 {
		spec := ledgerstore.WindowSpec{Rows: -since, Bytes: windowBytes, PreviewCap: previewCapOf(windowBytes)}
		if since, err = u.ledger.Window(ctx, chatID, spec, openTurns(st)); err != nil {
			return fmt.Errorf("chat feed: %s: %w", chatID, err)
		}
		from = since
	}
	err = u.replay(ctx, chatID, since, head, windowBytes > 0, func(r ledger.Row) bool {
		f := rowFrame(r)
		f.From, from = from, 0
		return emit(f)
	})
	if err != nil {
		return err
	}
	if !emit(FeedFrame{Type: FeedReady, Seq: head}) {
		return ctx.Err()
	}
	for _, d := range u.recorder.Snapshot(chatID, st) {
		if !emit(FeedFrame{Type: feedDelta, Delta: &d}) {
			return ctx.Err()
		}
	}
	return nil
}

func openTurns(st ledger.State) []string {
	var ids []string
	if t, ok := st.RunningTurn(); ok {
		ids = append(ids, t.ID)
	}
	for _, a := range st.PendingAsks() {
		ids = append(ids, a.TurnID)
	}
	for _, a := range st.Subagents() {
		ids = append(ids, a.TurnID)
	}
	for _, p := range st.PendingPrompts() {
		ids = append(ids, p.TurnID)
	}
	return ids
}

type feeds struct {
	capacity int
	mu       sync.Mutex
	byChat   map[string]map[*feedSub]struct{}
}

func newFeeds(capacity int) *feeds {
	return &feeds{capacity: capacity, byChat: map[string]map[*feedSub]struct{}{}}
}

type feedSub struct {
	wake    chan struct{}
	mu      sync.Mutex
	queue   []FeedFrame
	holding bool
	end     string
}

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

func (f *feeds) rows(chatID string, rows []ledger.Row) {
	for _, s := range f.subs(chatID) {
		for _, r := range rows {
			s.offer(rowFrame(r), f.capacity)
		}
	}
}

func (f *feeds) delta(chatID string, d ledger.FeedDelta) {
	for _, s := range f.subs(chatID) {
		s.offer(FeedFrame{Type: feedDelta, Delta: &d}, f.capacity)
	}
}

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

func (s *feedSub) pump(
	ctx context.Context, chatID string, out chan<- FeedFrame, since int64, attach func(emit func(FeedFrame) bool) error,
) {
	w := &feedWriter{ctx: ctx, out: out, last: since}
	if err := attach(w.emit); err != nil {
		if !errors.Is(err, errFeedEnded) && ctx.Err() == nil {
			slog.Warn("chat feed: attach", "chat_id", chatID, "err", err)
			w.emit(FeedFrame{Type: FeedFailed})
		}
		return
	}
	for s.step(w) {
	}
}

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
