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

// ── durable state: .buf files and meta rows ──────────────────────────────────
//
// Every write below runs under the entry's lifecycle lock and after checking the entry's
// state, which is what keeps disk in step with the state machine (invariant B3): nothing
// can persist a session after it ended, because ending happens under the same lock and
// deletes what persisting wrote. A command session (agentic vendor CLI) is never
// persisted: restore can only birth a login shell, so a row for one would come back as a
// bare shell wearing the agent's frame.

func (e *terminalEngine) meta() SessionMetaStore {
	e.cbMu.RLock()
	defer e.cbMu.RUnlock()
	return e.metaStore
}

// storageDir resolves the owning chat's scrollback directory, or "" with no meta store.
func (e *terminalEngine) storageDir(ctx context.Context, chatID string) (string, error) {
	ms := e.meta()
	if ms == nil {
		return "", nil
	}
	return ms.StorageDir(ctx, chatID)
}

// saveMeta upserts ent's meta row with the given state. Caller holds ent.mu.
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

// writeBuf persists a serialized screen for ent. It is a no-op (nil) without a storage
// directory. Caller holds ent.mu.
func (e *terminalEngine) writeBuf(ctx context.Context, ent *sessionEntry, blob []byte) error {
	dir, err := e.storageDir(ctx, ent.chatID)
	if err != nil || dir == "" {
		return err
	}
	return persistence.WriteBuf(dir, ent.id, blob)
}

// discardPersisted deletes ent's .buf and meta row. Caller holds ent.mu.
func (e *terminalEngine) discardPersisted(ctx context.Context, ent *sessionEntry) {
	if dir, err := e.storageDir(ctx, ent.chatID); err != nil {
		slog.Warn("terminal: resolve storage dir", "session", ent.id, "err", err)
	} else if dir != "" {
		if err := persistence.DeleteBuf(dir, ent.id); err != nil {
			slog.Warn("terminal: delete scrollback", "session", ent.id, "err", err)
		}
	}
	if ms := e.meta(); ms != nil {
		if err := ms.Delete(ctx, ent.id); err != nil {
			slog.Warn("terminal: delete session meta", "session", ent.id, "err", err)
		}
	}
}

// persistOnDetach records the last-known screen and a "detached" row when the final
// client leaves s, so a later restore resumes from it.
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
	e.fireState(ctx, ent.chatID, ent.id, "detached")
}

// flushOnce persists one live shell's screen if it changed since the last flush.
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
	// Snapshot consumes the dirty bit; an unchanged session skips the disk write (§8.4).
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

// ── bounds: the maintenance sweep ────────────────────────────────────────────

// maintenanceLoop runs runMaintenanceOnce on a ticker until Shutdown.
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

// candidate is one session seen by a sweep phase, with the facts the phase sorts and
// filters on. The facts are read under the entry lock; each action re-checks under it.
type candidate struct {
	ent        *sessionEntry
	lastActive time.Time
	state      sessionState
	// detached is a live shell with no client: the only kind a sweep may suspend.
	detached bool
	idle     bool
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

// runMaintenanceOnce performs the sweep in order:
//
//  1. Cadence flush — persist each live shell whose screen changed.
//  2. Per-chat soft limit — idle-gated-suspend the oldest idle detached shells of a chat
//     with more than softLimitPerChat detached shells.
//  3. Global ceiling — while over maxTotalSessions or maxTotalModelBytes: drop cached
//     blobs, then idle-gated-suspend, then force-suspend detached running shells, then
//     evict the oldest suspended sessions (the only step that lowers the count).
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
	fx := e.suspendLocked(ctx, ent, force)
	ent.mu.Unlock()
	fx.run()
}

// underCeiling counts EVERY registered session — live and suspended alike — and sums
// each one's resident bytes, so a pile of suspended sessions cannot slip past it.
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

	// Reclaimable cache first: ModelBytes counts each live session's cached blob, so the
	// ceiling can be tripped purely by cache (§9.4).
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
	// Last resort: suspending never lowers the COUNT, so once suspended sessions push us
	// over, the only way back under is to drop the least-recently-active ones.
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

// evict drops a Suspended session to reclaim its slot, .buf and meta row, and reports it
// ended — a session disappearing without an ended event would leave its tab waiting for a
// PTY that is never coming back. A session restored since it was picked is left alone.
func (e *terminalEngine) evict(ctx context.Context, ent *sessionEntry) {
	ent.mu.Lock()
	var fx effects
	if ent.state == stateSuspended {
		fx = e.endLocked(ctx, ent, stateRemoved, -1)
	}
	ent.mu.Unlock()
	fx.run()
}

// Stats returns a point-in-time snapshot of session counts, estimated model memory, and
// the parse-health surface (§9.4) across all registered sessions.
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
		}
		ent.mu.Unlock()
	}
	return active, detached, suspended, modelBytes, degraded, parsePanics
}
