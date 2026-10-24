// Package snapshot is the ONE owner of what a client is told a chat is: its
// row, the runner live on it, its phase, and a version that orders every
// answer the daemon gives about it (spec §7-A target 1, invariant A6).
//
// Every chat frame and every chat GET is built here, from the same in-memory
// state, under the same lock that assigns the version. So a snapshot with a
// higher version was read LATER, and describes a state at least as new — which
// is the only property a client needs to apply "newer version wins" and never
// guess again. The client-side read-ordering registry, list sequencing and
// reseed loops existed because reads were answered from projections that lag
// the events a frame had already announced; nothing read here lags an event,
// because the state is fed by the events themselves:
//
//   - the chat row comes from the chat aggregate carried ON its event (the
//     command-side fold, Working included), falling back to the read model only
//     for a chat no event has touched since boot;
//   - the live runner is read from the supervisor's table, and how the last
//     one ended from the chat's ledger.
//
// Memory is bounded by chats something has asked about; a deleted chat's
// entry goes with it (invariant A7).
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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

// KindSnapshot is the frame kind of a snapshot published for a change no
// aggregate event names — a phase, an attach, a terminal-wait verdict, or the
// chat a runner left.
const KindSnapshot = "snapshot"

// Snapshot is one versioned answer about a chat.
type Snapshot struct {
	Chat    domain.Chat
	Live    *agents.Runner
	Phase   string
	Version int64

	TerminalWait      domain.AgentTerminalWait
	AttachedSessionID string
	// Session is how the chat's last runner ended.
	Session domain.AgentSession
	Ledger  LedgerFacts
}

// Reader is the read model the owner falls back to for a chat no event has
// touched since boot.
type Reader interface {
	GetChat(ctx context.Context, chatID string) (domain.Chat, error)
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

// Frame is one published change: the snapshot, the event kind that caused it,
// and — for a runner event — the runner it was about.
type Frame struct {
	Snapshot Snapshot
	Kind     string
	RunnerID string
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
var ErrDeleted = fmt.Errorf("agent: chat snapshot: chat deleted: %w", apperr.ErrNotFound)

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
	// correct overlays what the chat row does not own — its tree placement,
	// which lives on the Node — onto the rows it is given, in order. It reads a
	// store, so a read applies it to all its rows at once and outside the lock.
	// Nil passes rows through.
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
func (s *Snapshots) ApplyChat(ctx context.Context, chat domain.Chat, version int64, kind string, forgotten bool) {
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
			Kind:     kind,
			Deleted:  true,
			Snapshot: Snapshot{Chat: domain.Chat{ID: chat.ID, WorkspaceID: chat.WorkspaceID}, Version: st.version},
		})
		return
	}
	st := s.stateLocked(chat.ID)
	if version >= st.chatVersion {
		st.chat, st.chatVersion, st.loaded = chat, version, true
	}
	s.emitChatLocked(ctx, chat.ID, kind, "")
}

// ApplyRunner publishes chatID's snapshot for a lifecycle event of runnerID:
// started or exited.
func (s *Snapshots) ApplyRunner(ctx context.Context, chatID, runnerID, kind string) {
	if chatID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitChatLocked(ctx, chatID, kind, runnerID)
}

// Touch publishes a fresh snapshot of chatID for a change no aggregate event
// carries. Callers must not hold a lock Runtime also takes.
func (s *Snapshots) Touch(ctx context.Context, chatID string) {
	s.Announce(ctx, chatID, KindSnapshot)
}

// Announce is Touch under a named kind — a tree placement written on the Node
// (a repo drag's collateral renumber), which no chat event carries but which
// the sidebar must hear about as the structural change it is.
func (s *Snapshots) Announce(ctx context.Context, chatID, kind string) {
	if chatID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitChatLocked(ctx, chatID, kind, "")
}

// Get returns chatID's current snapshot without advancing its version.
func (s *Snapshots) Get(ctx context.Context, chatID string) (Snapshot, error) {
	snaps, err := s.GetMany(ctx, []string{chatID})
	if err != nil {
		return Snapshot{}, err
	}
	return snaps[0], nil
}

// Held is chatID's snapshot from what the owner holds and the chat row, never
// reading the ledger: the read for a ledger subscriber, which runs under the
// chat's writer. It skips the placement overlay.
func (s *Snapshots) Held(ctx context.Context, chatID string) (Snapshot, error) {
	snaps, _, err := s.collect(ctx, []string{chatID})
	if err != nil {
		return Snapshot{}, err
	}
	return snaps[0], nil
}

// GetMany returns the current snapshot of every chat in chatIDs, in order,
// without advancing any version. The first chat that cannot be answered fails
// the read. The placement overlay runs once for the whole read, after the lock
// is released: it is a store read, and nothing it adds is versioned.
func (s *Snapshots) GetMany(ctx context.Context, chatIDs []string) ([]Snapshot, error) {
	if err := s.preloadLedger(ctx, chatIDs); err != nil {
		return nil, err
	}
	snaps, correct, err := s.collect(ctx, chatIDs)
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

// collect builds every snapshot, uncorrected, under one hold of the lock.
func (s *Snapshots) collect(
	ctx context.Context,
	chatIDs []string,
) ([]Snapshot, func(context.Context, []domain.Chat) []domain.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snaps := make([]Snapshot, len(chatIDs))
	for i, chatID := range chatIDs {
		if _, gone := s.deleted[chatID]; gone {
			return nil, nil, ErrDeleted
		}
		st := s.stateLocked(chatID)
		if err := s.loadLocked(ctx, chatID, st); err != nil {
			if !st.loaded {
				delete(s.chats, chatID)
			}
			return nil, nil, err
		}
		snaps[i] = s.buildLocked(chatID, st)
	}
	return snaps, s.correct, nil
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

func (s *Snapshots) emitChatLocked(ctx context.Context, chatID, kind, runnerID string) {
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
	s.emitLocked(Frame{Snapshot: snap, Kind: kind, RunnerID: runnerID})
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
	if snap.Live == nil && st.facts.LastExit != nil {
		snap.Session = domain.AgentSession{ExitReason: st.facts.LastExit.Reason, ExitedAt: st.facts.LastExit.At}
	}
	if snap.Phase == "" {
		snap.Phase = PhaseDormant
		if snap.Live != nil {
			snap.Phase = PhaseLive
		}
	}
	return snap
}
