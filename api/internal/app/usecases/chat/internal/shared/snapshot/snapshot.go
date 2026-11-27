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

const (
	PhaseDormant   = "dormant"
	PhaseStarting  = "starting"
	PhaseLive      = "live"
	PhaseSwitching = "switching"
	PhaseStopping  = "stopping"
)

type Snapshot struct {
	Chat    domain.Chat
	Live    *agents.Runner
	Phase   string
	Version int64

	TerminalWait      domain.AgentTerminalWait
	AttachedSessionID string
	Ledger            LedgerFacts
}

type Reader interface {
	GetChat(ctx context.Context, chatID string) (domain.Chat, error)
	GetChats(ctx context.Context, chatIDs []string) ([]domain.Chat, error)
	ViewLedger(ctx context.Context, chatID string, fn func(ledger.State) error) error
}

type Runtime interface {
	Phase(chatID string) string
	TerminalWait(chatID string) domain.AgentTerminalWait
	AttachedTerminalSession(runnerID string) (string, bool)
	LiveRunner(chatID string) (agents.Runner, bool)
}

type Frame struct {
	Snapshot Snapshot
	Deleted  bool
}

type Publish func(Frame)

type chatState struct {
	chat        domain.Chat
	chatVersion int64
	loaded      bool
	version     int64
	facts       LedgerFacts
	hasFacts    bool
}

var ErrDeleted = fmt.Errorf("agent: chat snapshot: chat deleted: %w", problem.ErrNotFound)

const deletedCap = 1024

type Snapshots struct {
	mu           sync.Mutex
	base         int64
	chats        map[string]*chatState
	deleted      map[string]struct{}
	deletedOrder []string
	reader       Reader
	runtime      Runtime
	publish      Publish
	correct      func(ctx context.Context, chats []domain.Chat) []domain.Chat
}

func New(base int64) *Snapshots {
	return &Snapshots{
		base:    base,
		chats:   map[string]*chatState{},
		deleted: map[string]struct{}{},
	}
}

func NewAtBoot() *Snapshots { return New(time.Now().UnixMicro()) }

func (s *Snapshots) SetPublish(publish Publish) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publish = publish
}

func (s *Snapshots) SetCorrect(correct func(ctx context.Context, chats []domain.Chat) []domain.Chat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.correct = correct
}

func (s *Snapshots) Bind(reader Reader, runtime Runtime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reader, s.runtime = reader, runtime
}

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
		if st.loaded {
			chat.ParentID, chat.Order = st.chat.ParentID, st.chat.Order
		}
		st.chat, st.chatVersion, st.loaded = chat, version, true
	}
	s.emitChatLocked(ctx, chat.ID)
}

func (s *Snapshots) Touch(ctx context.Context, chatID string) {
	if chatID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitChatLocked(ctx, chatID)
}

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

func (s *Snapshots) Get(ctx context.Context, chatID string) (Snapshot, error) {
	snaps, err := s.read(ctx, []string{chatID}, false)
	if err != nil {
		return Snapshot{}, err
	}
	return snaps[0], nil
}

func (s *Snapshots) Held(ctx context.Context, chatID string) (Snapshot, error) {
	snaps, _, err := s.collect(ctx, []string{chatID}, false)
	if err != nil {
		return Snapshot{}, err
	}
	return snaps[0], nil
}

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

func (s *Snapshots) Forget(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chats, chatID)
}

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
