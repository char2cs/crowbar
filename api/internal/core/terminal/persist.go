package terminal

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/persistence"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

func (e *terminalEngine) meta() SessionMetaStore {
	e.cbMu.RLock()
	defer e.cbMu.RUnlock()
	return e.metaStore
}

func (e *terminalEngine) storageDir(ctx context.Context, chatID string) (string, error) {
	ms := e.meta()
	if ms == nil {
		return "", nil
	}
	return ms.StorageDir(ctx, chatID)
}

func (e *terminalEngine) saveMeta(ctx context.Context, ent *sessionEntry, state string) {
	ms := e.meta()
	if ms == nil {
		return
	}
	cwd := ent.cwd
	if s := ent.sess.Load(); s != nil {
		cwd = s.CWD()
	}
	err := ms.Save(ctx, SessionMeta{
		SessionID:    ent.id,
		ChatID:       ent.chatID,
		CWD:          cwd,
		Shell:        ent.shell,
		ProfileID:    ent.profileID,
		State:        state,
		LastActiveAt: ent.lastActive,
	})
	if err != nil {
		slog.Warn("terminal: save session meta", "session", ent.id, "state", state, "err", err)
	}
}

func (e *terminalEngine) writeBuf(ctx context.Context, ent *sessionEntry, blob []byte) error {
	dir, err := e.storageDir(ctx, ent.chatID)
	if err != nil || dir == "" {
		return err
	}
	return persistence.WriteBuf(dir, ent.id, blob)
}

func (e *terminalEngine) discardPersisted(ctx context.Context, ent *sessionEntry) {
	e.deleteBuf(ctx, ent)
	if ms := e.meta(); ms != nil {
		if err := ms.Delete(ctx, ent.id); err != nil {
			slog.Warn("terminal: delete session meta", "session", ent.id, "err", err)
		}
	}
}

func (e *terminalEngine) deleteBuf(ctx context.Context, ent *sessionEntry) {
	dir, err := e.storageDir(ctx, ent.chatID)
	if err != nil {
		slog.Warn("terminal: resolve storage dir", "session", ent.id, "err", err)
		return
	}
	if dir == "" {
		return
	}
	if err := persistence.DeleteBuf(dir, ent.id); err != nil {
		slog.Warn("terminal: delete scrollback", "session", ent.id, "err", err)
	}
}

func (e *terminalEngine) persistOnDetach(ctx context.Context, ent *sessionEntry, s *session.Session) {
	ent.mu.Lock()
	if ent.state != stateLive || ent.sess.Load() != s || ent.command || s.AttachedCount() > 0 {
		ent.mu.Unlock()
		return
	}
	ent.lastActive = time.Now()
	blob, _ := s.Snapshot()
	if err := e.writeBuf(ctx, ent, blob); err != nil {
		slog.Warn("terminal: detach: persist scrollback", "session", ent.id, "err", err)
	}
	e.saveMeta(ctx, ent, "detached")
	ent.mu.Unlock()
}

func (e *terminalEngine) flushOnce(ctx context.Context, ent *sessionEntry) {
	ent.mu.Lock()
	defer ent.mu.Unlock()
	if ent.state != stateLive || ent.command {
		return
	}
	dir, err := e.storageDir(ctx, ent.chatID)
	if err != nil || dir == "" {
		return
	}
	blob, changed := ent.sess.Load().Snapshot()
	if !changed {
		return
	}
	if err := persistence.WriteBuf(dir, ent.id, blob); err != nil {
		slog.Warn("terminal: maintenance: flush scrollback", "session", ent.id, "err", err)
		return
	}
	e.saveMeta(ctx, ent, ent.stateStringLocked())
}

func (e *terminalEngine) maintenanceLoop() {
	defer safego.Recover("terminal.maintenanceLoop")
	defer close(e.maintDone)
	ticker := time.NewTicker(e.cfg.maintenanceTick)
	defer ticker.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			e.runMaintenanceOnce(context.Background())
		}
	}
}

type candidate struct {
	ent        *sessionEntry
	lastActive time.Time
	state      sessionState
	detached   bool
	idle       bool
}

func (e *terminalEngine) candidates() []candidate {
	ents := e.snapshot()
	out := make([]candidate, 0, len(ents))
	for _, ent := range ents {
		ent.mu.Lock()
		c := candidate{ent: ent, lastActive: ent.lastActive, state: ent.state}
		if ent.state == stateLive && !ent.command && ent.sess.Load().AttachedCount() == 0 {
			c.detached = true
			c.idle = ent.sess.Load().IsIdle()
		}
		ent.mu.Unlock()
		out = append(out, c)
	}
	return out
}

func oldestFirst(cs []candidate) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].lastActive.Before(cs[j].lastActive) })
}

func (e *terminalEngine) runMaintenanceOnce(ctx context.Context) {
	for _, ent := range e.snapshot() {
		e.flushOnce(ctx, ent)
	}

	byChat := make(map[string][]candidate)
	for _, c := range e.candidates() {
		if c.detached {
			byChat[c.ent.chatID] = append(byChat[c.ent.chatID], c)
		}
	}
	for _, detached := range byChat {
		excess := len(detached) - e.cfg.softLimitPerChat
		if excess <= 0 {
			continue
		}
		var idle []candidate
		for _, c := range detached {
			if c.idle {
				idle = append(idle, c)
			}
		}
		oldestFirst(idle)
		for i := 0; i < excess && i < len(idle); i++ {
			e.suspendEntry(ctx, idle[i].ent, false)
		}
	}

	e.enforceCeiling(ctx)
}

func (e *terminalEngine) suspendEntry(ctx context.Context, ent *sessionEntry, force bool) {
	ent.mu.Lock()
	e.suspendLocked(ctx, ent, force)
	ent.mu.Unlock()
}

func (e *terminalEngine) underCeiling() bool {
	ents := e.snapshot()
	var bytes int64
	for _, ent := range ents {
		ent.mu.Lock()
		switch ent.state {
		case stateLive:
			bytes += ent.sess.Load().ModelBytes()
		case stateSuspended:
			bytes += int64(len(ent.blob))
		case stateExited, stateRemoved:
		}
		ent.mu.Unlock()
	}
	return len(ents) <= e.cfg.maxTotalSessions && bytes <= e.cfg.maxTotalModelBytes
}

func (e *terminalEngine) enforceCeiling(ctx context.Context) {
	if e.underCeiling() {
		return
	}
	cs := e.candidates()
	oldestFirst(cs)

	for _, c := range cs {
		if s := c.ent.live(); s != nil {
			s.DropCachedBlob()
			if e.underCeiling() {
				return
			}
		}
	}
	for _, force := range []bool{false, true} {
		for _, c := range cs {
			if !c.detached || (!force && !c.idle) {
				continue
			}
			e.suspendEntry(ctx, c.ent, force)
			if e.underCeiling() {
				return
			}
		}
	}
	evictable := e.candidates()
	oldestFirst(evictable)
	for _, c := range evictable {
		if e.underCeiling() {
			return
		}
		if c.state == stateSuspended {
			e.evict(ctx, c.ent)
		}
	}
}

func (e *terminalEngine) evict(ctx context.Context, ent *sessionEntry) {
	ent.mu.Lock()
	var fx effects
	if ent.state == stateSuspended {
		fx = e.endLocked(ctx, ent, stateRemoved, -1)
	}
	ent.mu.Unlock()
	fx.run()
}

func (e *terminalEngine) Stats() (active, detached, suspended int, modelBytes int64, degraded, parsePanics int) {
	for _, ent := range e.snapshot() {
		ent.mu.Lock()
		switch ent.state {
		case stateLive:
			modelBytes += ent.sess.Load().ModelBytes()
			dg, pp := ent.sess.Load().Health()
			if dg {
				degraded++
			}
			parsePanics += pp
			if ent.sess.Load().AttachedCount() > 0 {
				active++
			} else {
				detached++
			}
		case stateSuspended:
			modelBytes += int64(len(ent.blob))
			suspended++
		case stateExited, stateRemoved:
		}
		ent.mu.Unlock()
	}
	return active, detached, suspended, modelBytes, degraded, parsePanics
}
