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
//   - runner placement comes from the runner aggregates carried on their events,
//     seeded once from the live-runner read model at boot.
//
// Memory is bounded by live runners and by chats something has asked about;
// a deleted chat's entry goes with it (invariant A7).
package snapshot

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Phases a chat is in. Live and dormant are derived from placement; the other
// three are set by the lifecycle operation holding the chat's spawn gate.
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
	// Session is why the chat's vendor session is in the state it is.
	Session domain.AgentSession
}

// Reader is the read model the owner falls back to for a chat no event has
// touched since boot, and seeds live placement from.
type Reader interface {
	GetChat(ctx context.Context, chatID string) (domain.Chat, error)
	AllLive(ctx context.Context) ([]agents.Runner, error)
}

// Runtime is the in-memory process state joined onto a snapshot. Its
// implementations must never call back into Snapshots while holding a lock of
// their own.
type Runtime interface {
	Phase(chatID string) string
	TerminalWait(chatID string) domain.AgentTerminalWait
	AttachedTerminalSession(runnerID string) (string, bool)
	Session(chatID string) domain.AgentSession
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

type runnerState struct {
	runner  agents.Runner
	version int64
}

type chatState struct {
	chat        domain.Chat
	chatVersion int64
	loaded      bool
	version     int64
}

// Snapshots owns every chat's versioned snapshot.
type Snapshots struct {
	mu      sync.Mutex
	base    int64
	chats   map[string]*chatState
	runners map[string]runnerState
	// exited remembers runners that left before the boot seed ran, so the seed
	// never resurrects one from a read model that has not caught up. Dropped
	// once seeded.
	exited  map[string]struct{}
	seeded  bool
	reader  Reader
	runtime Runtime
	publish Publish
	// correct overlays what the chat row does not own — its tree placement,
	// which lives on the Node — onto every snapshot. Nil passes rows through.
	correct func(ctx context.Context, chat domain.Chat) domain.Chat
}

// New returns an empty owner. base is the first version it hands out; the
// daemon passes its boot time in microseconds so a restarted daemon's versions
// are larger than any the previous one issued.
func New(base int64) *Snapshots {
	return &Snapshots{
		base:    base,
		chats:   map[string]*chatState{},
		runners: map[string]runnerState{},
		exited:  map[string]struct{}{},
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
func (s *Snapshots) SetCorrect(correct func(ctx context.Context, chat domain.Chat) domain.Chat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.correct = correct
}

// Bind wires the read model and runtime. Live placement is seeded from the
// read model once, on first use; events applied before that are kept — they
// are newer than anything the seed reads.
func (s *Snapshots) Bind(reader Reader, runtime Runtime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reader, s.runtime = reader, runtime
}

// seedLocked reads live placement from the read model the first time a
// snapshot is built. A failed read is retried on the next one.
func (s *Snapshots) seedLocked(ctx context.Context) {
	if s.seeded || s.reader == nil {
		return
	}
	live, err := s.reader.AllLive(ctx)
	if err != nil {
		return
	}
	for _, r := range live {
		if _, known := s.runners[r.ID]; known {
			continue
		}
		if _, gone := s.exited[r.ID]; gone {
			continue
		}
		s.runners[r.ID] = runnerState{runner: r}
	}
	s.exited = nil
	s.seeded = true
}

// ApplyChat records one chat aggregate event and publishes its snapshot.
func (s *Snapshots) ApplyChat(ctx context.Context, chat domain.Chat, version int64, kind string, forgotten bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if forgotten {
		st := s.stateLocked(chat.ID)
		st.version++
		delete(s.chats, chat.ID)
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

// ApplyRunner records one runner aggregate event and publishes the snapshot of
// every chat whose placement it changed: the chat it is on now, and the one it
// left.
func (s *Snapshots) ApplyRunner(ctx context.Context, runner, previous agents.Runner, version int64, kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.runners[runner.ID]; ok && version != 0 && cur.version > version {
		return // an older event of this runner, delivered late
	}
	s.recordRunnerLocked(runner, version)
	if runner.CurrentChatID != "" {
		s.emitChatLocked(ctx, runner.CurrentChatID, kind, runner.ID)
	}
	if left := previous.CurrentChatID; left != "" && left != runner.CurrentChatID {
		s.emitChatLocked(ctx, left, KindSnapshot, runner.ID)
	}
}

// recordRunnerLocked keeps a live runner at version and forgets an exited one — remembering
// its exit until the seed, so the seed cannot resurrect it. Caller holds s.mu.
func (s *Snapshots) recordRunnerLocked(runner agents.Runner, version int64) {
	if runner.ExitedAt == nil {
		s.runners[runner.ID] = runnerState{runner: runner, version: version}
		return
	}
	delete(s.runners, runner.ID)
	if !s.seeded {
		s.exited[runner.ID] = struct{}{}
	}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stateLocked(chatID)
	if err := s.loadLocked(ctx, chatID, st); err != nil {
		if !st.loaded {
			delete(s.chats, chatID)
		}
		return Snapshot{}, err
	}
	return s.buildLocked(ctx, chatID, st), nil
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
	s.emitLocked(Frame{Snapshot: s.buildLocked(ctx, chatID, st), Kind: kind, RunnerID: runnerID})
}

func (s *Snapshots) emitLocked(f Frame) {
	if s.publish != nil {
		s.publish(f)
	}
}

func (s *Snapshots) buildLocked(ctx context.Context, chatID string, st *chatState) Snapshot {
	s.seedLocked(ctx)
	snap := Snapshot{Chat: st.chat, Version: st.version, Live: s.liveLocked(chatID)}
	if s.correct != nil {
		snap.Chat = s.correct(ctx, snap.Chat)
	}
	if s.runtime != nil {
		snap.Phase = s.runtime.Phase(chatID)
		snap.TerminalWait = s.runtime.TerminalWait(chatID)
		snap.Session = s.runtime.Session(chatID)
		if snap.Live != nil {
			snap.AttachedSessionID, _ = s.runtime.AttachedTerminalSession(snap.Live.ID)
		}
	}
	if snap.Phase == "" {
		snap.Phase = PhaseDormant
		if snap.Live != nil {
			snap.Phase = PhaseLive
		}
	}
	return snap
}

// liveLocked is the runner the single-chat read model would name: newest
// arrival first, id ascending on a tie.
func (s *Snapshots) liveLocked(chatID string) *agents.Runner {
	var placed []agents.Runner
	for _, rs := range s.runners {
		if rs.runner.CurrentChatID == chatID {
			placed = append(placed, rs.runner)
		}
	}
	if len(placed) == 0 {
		return nil
	}
	sort.Slice(placed, func(i, j int) bool {
		ai, aj := arrival(placed[i]), arrival(placed[j])
		if !ai.Equal(aj) {
			return ai.After(aj)
		}
		return placed[i].ID < placed[j].ID
	})
	return &placed[0]
}

func arrival(r agents.Runner) time.Time {
	if r.CurrentSessionSince.After(r.StartedAt) {
		return r.CurrentSessionSince
	}
	return r.StartedAt
}
