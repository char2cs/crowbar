// Package runner (file supervisor.go) is the session supervisor's record of
// WHY each chat's session is in the state it is: which resume rung its runner
// launched on, and how its last runner ended. The snapshot joins it onto every
// chat frame, so a client renders the reason and never infers it.
package runner

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// sessionBook is in memory only, like every live-process registry here: a
// daemon restart is itself recorded (AgentExitDaemonRestart) at boot. A nil
// book (a bare Runners built by a test) records nothing.
type sessionBook struct {
	mu    sync.Mutex
	chats map[string]domain.AgentSession
	// causes is the reason noted for a runner BEFORE Crowbar ends it, read
	// once by the exit that produces. A runner with none exited on its own.
	causes map[string]string
}

func newSessionBook() *sessionBook {
	return &sessionBook{chats: map[string]domain.AgentSession{}, causes: map[string]string{}}
}

func (b *sessionBook) get(chatID string) domain.AgentSession {
	if b == nil {
		return domain.AgentSession{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.chats[chatID]
}

func (b *sessionBook) set(chatID string, s domain.AgentSession) {
	if b == nil || chatID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chats[chatID] = s
}

func (b *sessionBook) cause(runnerID, reason string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.causes[runnerID] = reason
}

// takeCause consumes runnerID's noted cause; "exited" when none was noted.
func (b *sessionBook) takeCause(runnerID string) string {
	if b == nil {
		return domain.AgentExitExited
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	reason, noted := b.causes[runnerID]
	delete(b.causes, runnerID)
	if !noted {
		return domain.AgentExitExited
	}
	return reason
}

func (b *sessionBook) forget(chatID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.chats, chatID)
}

// Session implements snapshot.Runtime.
func (rs *Runners) Session(chatID string) domain.AgentSession {
	return rs.sessions.get(chatID)
}

// noteLaunch records that chatID's new runner went live on rung.
func (rs *Runners) noteLaunch(ctx context.Context, chatID, rung string) {
	rs.sessions.set(chatID, domain.AgentSession{Rung: rung})
	rs.touch(ctx, chatID)
}

// noteExit records how chatID's runner (runnerID) ended, consuming the cause
// noted for it. A runner already off its chat (displaced) records nothing.
func (rs *Runners) noteExit(ctx context.Context, chatID, runnerID string) {
	reason := rs.sessions.takeCause(runnerID)
	if chatID == "" {
		return
	}
	rs.noteChatExit(ctx, chatID, reason)
}

// noteChatExit records a reason on chatID directly, for an end no single
// runner exit carries (a stop, a failed spawn, a daemon restart).
func (rs *Runners) noteChatExit(ctx context.Context, chatID, reason string) {
	rs.sessions.set(chatID, domain.AgentSession{ExitReason: reason, ExitedAt: time.Now()})
	rs.touch(ctx, chatID)
}

// launchRung is how a spawn continues its chat: the provider's own session,
// Crowbar's transcript handed to a fresh one, or nothing to continue.
func launchRung(resuming bool, conversation string) string {
	switch {
	case resuming:
		return domain.AgentRungSession
	case conversation != "":
		return domain.AgentRungTranscript
	default:
		return domain.AgentRungFresh
	}
}
