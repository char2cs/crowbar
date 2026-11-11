// Package snapshot is the one owner of what a client is told a chat is: row, live runner, phase, and a version ordering
// every answer. Frames and GETs are built from one in-memory state under the lock that assigns the version, so "newer
// version wins" is safe. Events feed that state directly, so it never lags a frame; the read model is only a fallback
// for chats no event has touched since boot. Memory is bounded by chats asked about.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Phases a chat is in. Live and dormant are derived from the live runner; the other
// three are set by the lifecycle operation holding the chat.
const (
	PhaseDormant   = "dormant"
	PhaseStarting  = "starting"
	PhaseLive      = "live"
	PhaseSwitching = "switching"
	PhaseStopping  = "stopping"
)

// Snapshot is one versioned answer about a chat.
type Snapshot struct {
	Chat    domain.Chat
	Live    *agents.Runner
	Phase   string
	Version int64

	TerminalWait      domain.AgentTerminalWait
	AttachedSessionID string
	Ledger            LedgerFacts
}

// Reader is the read model the owner falls back to for a chat no event has
// touched since boot.
type Reader interface {
	GetChat(ctx context.Context, chatID string) (domain.Chat, error)
	// GetChats is every chat of chatIDs that exists, in one read.
	GetChats(ctx context.Context, chatIDs []string) ([]domain.Chat, error)
	// ViewLedger runs fn on the chat's ledger State under its writer. It is
	// never called under the owner's lock: the writer publishes into the owner.
	ViewLedger(ctx context.Context, chatID string, fn func(ledger.State) error) error
}

// Runtime is the in-memory process state joined onto a snapshot. Its
// implementations must never call back into Snapshots while holding a lock of
// their own.
type Runtime interface {
	Phase(chatID string) string
	TerminalWait(chatID string) domain.AgentTerminalWait
	AttachedTerminalSession(runnerID string) (string, bool)
	LiveRunner(chatID string) (agents.Runner, bool)
}

// Frame is one published change: the chat's snapshot as of it.
type Frame struct {
	Snapshot Snapshot
	// Deleted is a chat that no longer exists; Snapshot then carries only its
	// id, workspace and the version that retires it.
	Deleted bool
}

// Publish sends a frame to clients. It is called under the owner's lock, so
// frames for one chat leave in version order; it must not block.
type Publish func(Frame)

type chatState struct {
	chat        domain.Chat
	chatVersion int64
	loaded      bool
	version     int64
	facts       LedgerFacts
	hasFacts    bool
}

// ErrDeleted is a chat whose delete this owner has applied.
var ErrDeleted = fmt.Errorf("agent: chat snapshot: chat deleted: %w", problem.ErrNotFound)

// deletedCap bounds the deleted-chat memory: a late event trails its delete
// by the fan-out's goroutine scheduling, not by a thousand later deletes.
const deletedCap = 1024

// Snapshots owns every chat's versioned snapshot.
type Snapshots struct {
	mu    sync.Mutex
	base  int64
	chats map[string]*chatState
	// deleted remembers recent deletes, oldest first: events are delivered
	// concurrently, so one older than a delete can land after it, over a read
	// model that may not have caught up, and must not resurrect the chat.
	deleted      map[string]struct{}
	deletedOrder []string
	reader       Reader
	runtime      Runtime
	publish      Publish
	// correct overlays the tree placement (owned by the Node, not the chat row) onto rows, in order.
	// It reads a store, so it runs once per read, outside the lock. Nil passes rows through.
	correct func(ctx context.Context, chats []domain.Chat) []domain.Chat
}

// New returns an empty owner. base is the first version it hands out; the
// daemon passes its boot time in microseconds so a restarted daemon's versions
// are larger than any the previous one issued.
func New(base int64) *Snapshots {
	return &Snapshots{
		base:    base,
		chats:   map[string]*chatState{},
		deleted: map[string]struct{}{},
	}
}

// NewAtBoot is New with the conventional base: now, in microseconds.
func NewAtBoot() *Snapshots { return New(time.Now().UnixMicro()) }

// SetPublish binds where frames go. Before it is bound, changes are recorded
// and not sent.
func (s *Snapshots) SetPublish(publish Publish) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publish = publish
}

// SetCorrect binds the overlay applied to every snapshot's row.
func (s *Snapshots) SetCorrect(correct func(ctx context.Context, chats []domain.Chat) []domain.Chat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.correct = correct
}

// Bind wires the read model and runtime.
func (s *Snapshots) Bind(reader Reader, runtime Runtime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reader, s.runtime = reader, runtime
}

// ApplyChat records one chat aggregate event and publishes its snapshot.
func (s *Snapshots) ApplyChat(ctx context.Context, chat domain.Chat, version int64, forgotten bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, gone := s.deleted[chat.ID]; gone {
		return
	}
	if forgotten {
		st := s.stateLocked(chat.ID)
		st.version++
		delete(s.chats, chat.ID)
		s.rememberDeletedLocked(chat.ID)
		s.emitLocked(Frame{
			Deleted:  true,
			Snapshot: Snapshot{Chat: domain.Chat{ID: chat.ID, WorkspaceID: chat.WorkspaceID}, Version: st.version},
		})
		return
	}
	st := s.stateLocked(chat.ID)
	if version >= st.chatVersion {
		// Placement is the Tree's, announced through ApplyPlacement; the event's
		// copy was read when it was raised and may predate a move.
		if st.loaded {
			chat.ParentID, chat.Order = st.chat.ParentID, st.chat.Order
		}
		st.chat, st.chatVersion, st.loaded = chat, version, true
	}
	s.emitChatLocked(ctx, chat.ID)
}

// Touch publishes a fresh snapshot of chatID for a change no aggregate event
// carries: a phase, a runner starting or exiting, a Node placement written as
// a drag's collateral. Callers must not hold a lock Runtime also takes.
func (s *Snapshots) Touch(ctx context.Context, chatID string) {
	if chatID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitChatLocked(ctx, chatID)
}

// ApplyPlacement records chatID's new place in the tree, which no chat event
// carries, and publishes its snapshot. The Tree announces a write
// before every reader sees it, so the placement given wins over a load's row.
func (s *Snapshots) ApplyPlacement(ctx context.Context, chatID, parentID string, order int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, gone := s.deleted[chatID]; gone {
		return
	}
	st := s.stateLocked(chatID)
	if err := s.loadLocked(ctx, chatID, st); err != nil {
		if !st.loaded {
			delete(s.chats, chatID)
		}
		return
	}
	st.chat.ParentID, st.chat.Order = parentID, order
	s.emitChatLocked(ctx, chatID)
}

// Get returns chatID's current snapshot without advancing its version.
func (s *Snapshots) Get(ctx context.Context, chatID string) (Snapshot, error) {
	snaps, err := s.read(ctx, []string{chatID}, false)
	if err != nil {
		return Snapshot{}, err
	}
	return snaps[0], nil
}

// Held is chatID's snapshot from what the owner holds and the chat row, never
// reading the ledger: the read for a ledger subscriber, which runs under the
// chat's writer. It skips the placement overlay.
func (s *Snapshots) Held(ctx context.Context, chatID string) (Snapshot, error) {
	snaps, _, err := s.collect(ctx, []string{chatID}, false)
	if err != nil {
		return Snapshot{}, err
	}
	return snaps[0], nil
}

// GetMany is every chat of chatIDs' current snapshot, in order; a chat deleted
// since it was listed is left out, any other that cannot be answered fails it.
// The placement overlay runs once, after the lock: nothing it adds is versioned.
func (s *Snapshots) GetMany(ctx context.Context, chatIDs []string) ([]Snapshot, error) {
	return s.read(ctx, chatIDs, true)
}

func (s *Snapshots) read(ctx context.Context, chatIDs []string, skipGone bool) ([]Snapshot, error) {
	if err := s.preloadLedger(ctx, chatIDs); err != nil {
		return nil, err
	}
	if err := s.preloadChats(ctx, chatIDs); err != nil {
		return nil, err
	}
	snaps, correct, err := s.collect(ctx, chatIDs, skipGone)
	if err != nil || correct == nil {
		return snaps, err
	}
	chats := make([]domain.Chat, len(snaps))
	for i, snap := range snaps {
		chats[i] = snap.Chat
	}
	for i, chat := range correct(ctx, chats) {
		snaps[i].Chat = chat
	}
	return snaps, nil
}

// preloadChats reads every chat of chatIDs the owner does not hold in one read,
// outside the lock. An event that lands meanwhile loads its chat first and wins.
func (s *Snapshots) preloadChats(ctx context.Context, chatIDs []string) error {
	s.mu.Lock()
	reader := s.reader
	var missing []string
	for _, chatID := range chatIDs {
		_, gone := s.deleted[chatID]
		if st := s.chats[chatID]; !gone && (st == nil || !st.loaded) {
			missing = append(missing, chatID)
		}
	}
	s.mu.Unlock()
	if reader == nil || len(missing) == 0 {
		return nil
	}
	chats, err := reader.GetChats(ctx, missing)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, chat := range chats {
		if _, gone := s.deleted[chat.ID]; gone {
			continue
		}
		if st := s.stateLocked(chat.ID); !st.loaded {
			st.chat, st.loaded = chat, true
		}
	}
	return nil
}

// collect builds every snapshot, uncorrected, under one hold of the lock;
// skipGone leaves out a chat deleted or no longer found instead of failing.
func (s *Snapshots) collect(
	ctx context.Context,
	chatIDs []string,
	skipGone bool,
) ([]Snapshot, func(context.Context, []domain.Chat) []domain.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snaps := make([]Snapshot, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		snap, err := s.collectLocked(ctx, chatID)
		if skipGone && problem.CodeOf(err) == problem.NotFound {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		snaps = append(snaps, snap)
	}
	return snaps, s.correct, nil
}

func (s *Snapshots) collectLocked(ctx context.Context, chatID string) (Snapshot, error) {
	if _, gone := s.deleted[chatID]; gone {
		return Snapshot{}, ErrDeleted
	}
	st := s.stateLocked(chatID)
	if err := s.loadLocked(ctx, chatID, st); err != nil {
		if !st.loaded {
			delete(s.chats, chatID)
		}
		return Snapshot{}, err
	}
	return s.buildLocked(chatID, st), nil
}

// Forget drops chatID's entry — a chat a client asked about that turned out
// not to exist, or one deleted without an event reaching here.
func (s *Snapshots) Forget(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chats, chatID)
}

// Len is how many chats hold an entry.
func (s *Snapshots) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.chats)
}

func (s *Snapshots) rememberDeletedLocked(chatID string) {
	s.deleted[chatID] = struct{}{}
	s.deletedOrder = append(s.deletedOrder, chatID)
	if len(s.deletedOrder) > deletedCap {
		delete(s.deleted, s.deletedOrder[0])
		s.deletedOrder = s.deletedOrder[1:]
	}
}

func (s *Snapshots) stateLocked(chatID string) *chatState {
	st := s.chats[chatID]
	if st == nil {
		st = &chatState{version: s.base}
		s.chats[chatID] = st
	}
	return st
}

// loadLocked fills a chat no event has delivered from the read model. The
// read runs under the lock: it is one row, and an event racing it must not
// be overwritten by the older answer.
func (s *Snapshots) loadLocked(ctx context.Context, chatID string, st *chatState) error {
	if st.loaded {
		return nil
	}
	if s.reader == nil {
		return errors.New("agent: snapshots: not bound")
	}
	chat, err := s.reader.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	st.chat, st.loaded = chat, true
	return nil
}

func (s *Snapshots) emitChatLocked(ctx context.Context, chatID string) {
	if _, gone := s.deleted[chatID]; gone {
		return
	}
	st := s.stateLocked(chatID)
	if err := s.loadLocked(ctx, chatID, st); err != nil {
		// Nothing to describe: an event for a chat the read model has never
		// seen (a runner pointed at a chat mid-delete). Drop the entry rather
		// than hold an empty one.
		if !st.loaded {
			delete(s.chats, chatID)
		}
		return
	}
	st.version++
	snap := s.buildLocked(chatID, st)
	if s.correct != nil {
		snap.Chat = s.correct(ctx, []domain.Chat{snap.Chat})[0]
	}
	s.emitLocked(Frame{Snapshot: snap})
}

func (s *Snapshots) emitLocked(f Frame) {
	if s.publish != nil {
		s.publish(f)
	}
}

// applyRuntimeLocked fills in what only the running daemon knows about chatID.
func (s *Snapshots) applyRuntimeLocked(chatID string, snap *Snapshot) {
	snap.Phase = s.runtime.Phase(chatID)
	snap.TerminalWait = s.runtime.TerminalWait(chatID)
	live, ok := s.runtime.LiveRunner(chatID)
	if !ok {
		return
	}
	snap.Live = &live
	snap.AttachedSessionID, _ = s.runtime.AttachedTerminalSession(live.ID)
}

func (s *Snapshots) buildLocked(chatID string, st *chatState) Snapshot {
	snap := Snapshot{Chat: st.chat, Version: st.version, Ledger: st.facts}
	if s.runtime != nil {
		s.applyRuntimeLocked(chatID, &snap)
	}
	if snap.Phase == "" {
		snap.Phase = PhaseDormant
		if snap.Live != nil {
			snap.Phase = PhaseLive
		}
	}
	return snap
}
