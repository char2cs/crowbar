package terminal

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

func (e *terminalEngine) lookup(id string) (*sessionEntry, bool) {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	ent, ok := e.entries[id]
	return ent, ok
}

func (e *terminalEngine) register(ent *sessionEntry) {
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.registerLocked(ent)
}

func (e *terminalEngine) registerLocked(ent *sessionEntry) {
	e.entries[ent.id] = ent
	ids := e.byChat[ent.chatID]
	if ids == nil {
		ids = make(map[string]*sessionEntry)
		e.byChat[ent.chatID] = ids
	}
	ids[ent.id] = ent
}

func (e *terminalEngine) unregister(ent *sessionEntry) {
	e.regMu.Lock()
	defer e.regMu.Unlock()
	if e.entries[ent.id] != ent {
		return
	}
	delete(e.entries, ent.id)
	ids := e.byChat[ent.chatID]
	delete(ids, ent.id)
	if len(ids) == 0 {
		delete(e.byChat, ent.chatID)
	}
}

func (e *terminalEngine) snapshot() []*sessionEntry {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	out := make([]*sessionEntry, 0, len(e.entries))
	for _, ent := range e.entries {
		out = append(out, ent)
	}
	return out
}

func (e *terminalEngine) ListSessions() []string {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	ids := make([]string, 0, len(e.entries))
	for id := range e.entries {
		ids = append(ids, id)
	}
	return ids
}

func (e *terminalEngine) ListSessionsForChat(chatID string) []string {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	ids := e.byChat[chatID]
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

func (ent *sessionEntry) live() *session.Session {
	return ent.sess.Load()
}

func (e *terminalEngine) liveSession(id string) *session.Session {
	ent, ok := e.lookup(id)
	if !ok {
		return nil
	}
	return ent.live()
}

func (ent *sessionEntry) stateStringLocked() string {
	switch ent.state {
	case stateLive:
		if ent.sess.Load().AttachedCount() > 0 {
			return "active"
		}
		return "detached"
	case stateSuspended:
		return "suspended"
	case stateExited, stateRemoved:
		return ""
	}
	return ""
}

func (e *terminalEngine) StateOf(sessionID string) (string, bool) {
	ent, ok := e.lookup(sessionID)
	if !ok {
		return "", false
	}
	ent.mu.Lock()
	defer ent.mu.Unlock()
	st := ent.stateStringLocked()
	return st, st != ""
}

func (e *terminalEngine) SessionExists(_ context.Context, sessionID string) bool {
	_, ok := e.lookup(sessionID)
	return ok
}

func (e *terminalEngine) SessionLive(_ context.Context, sessionID string) bool {
	s := e.liveSession(sessionID)
	if s == nil {
		return false
	}
	select {
	case <-s.Done():
		return false
	default:
		return true
	}
}

func (e *terminalEngine) Screen(sessionID string, since uint64) (text string, gen uint64, changed bool) {
	s := e.liveSession(sessionID)
	if s == nil {
		return "", 0, false
	}
	return s.ScreenText(since)
}

func (e *terminalEngine) ScreenState(sessionID string) (ScreenState, error) {
	s := e.liveSession(sessionID)
	if s == nil {
		return ScreenState{}, fmt.Errorf("terminal: screen state: %w: %s", ErrSessionNotFound, sessionID)
	}
	st, ok := s.ScreenState()
	if !ok {
		return ScreenState{}, fmt.Errorf("terminal: screen state: %w: %s", ErrScreenStateUnavailable, sessionID)
	}
	return st, nil
}

func (e *terminalEngine) Write(ctx context.Context, sessionID string, data []byte) error {
	s := e.liveSession(sessionID)
	if s == nil {
		return fmt.Errorf("terminal: write: %w: %s", ErrSessionNotFound, sessionID)
	}
	if err := context.Cause(ctx); err != nil {
		return fmt.Errorf("terminal: write: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Write(data) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("terminal: write: %w", context.Cause(ctx))
	}
}

func (e *terminalEngine) Resize(_ context.Context, sessionID string, cols, rows uint16) error {
	s := e.liveSession(sessionID)
	if s == nil {
		return fmt.Errorf("terminal: resize: %w: %s", ErrSessionNotFound, sessionID)
	}
	return s.Resize(cols, rows)
}

func (e *terminalEngine) LoadPlaceholder(_ context.Context, m SessionMeta, scrollback []byte) error {
	e.regMu.Lock()
	defer e.regMu.Unlock()
	if _, ok := e.entries[m.SessionID]; ok {
		return nil
	}
	ent := &sessionEntry{
		id:         m.SessionID,
		chatID:     m.ChatID,
		state:      stateSuspended,
		shell:      m.Shell,
		cwd:        m.CWD,
		profileID:  m.ProfileID,
		blob:       scrollback,
		lastActive: m.LastActiveAt,
	}
	e.registerLocked(ent)
	return nil
}
