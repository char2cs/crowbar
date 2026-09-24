// Package runner (file supervisor.go) is the session supervisor's record of
// WHY each chat's session is in the state it is: which resume rung its runner
// launched on, and how its last runner ended. The snapshot joins it onto every
// chat frame, so a client renders the reason and never infers it.
package runner

import (
	"context"
	"errors"
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
	// probes are resume launches not yet confirmed by the CLI announcing a
	// session; one that exits unconfirmed failed its resume (see ladder.go).
	probes map[string]resumeProbe
	// quarantined is, per chat, the vendor session ids a launch failed to
	// resume: the ladder never offers them again.
	quarantined map[string]map[string]struct{}
}

// backgroundWork is the supervisor's own follow-up work (a refused prompt's
// redelivery): tracked, so Shutdown cancels it and waits, and none outlives
// the daemon. A nil one (a bare Runners built by a test) runs untracked.
type backgroundWork struct {
	mu      sync.Mutex
	stopped bool
	cancels map[*context.CancelFunc]struct{}
	wg      sync.WaitGroup
}

func newBackgroundWork() *backgroundWork {
	return &backgroundWork{cancels: map[*context.CancelFunc]struct{}{}}
}

// run starts fn with parent's values but the supervisor's lifetime: it
// outlives the request that asked for it and ends with Shutdown.
func (b *backgroundWork) run(parent context.Context, fn func(context.Context)) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	if b == nil {
		go func() {
			defer cancel()
			fn(ctx)
		}()
		return
	}
	b.mu.Lock()
	if b.stopped {
		cancel() // after Shutdown it runs already cancelled, and is still waited for
	}
	b.cancels[&cancel] = struct{}{}
	b.wg.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.wg.Done()
		defer b.forget(&cancel)
		fn(ctx)
	}()
}

func (b *backgroundWork) forget(cancel *context.CancelFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.cancels, cancel)
	(*cancel)()
}

// stop cancels every piece of background work and waits for it to return.
func (b *backgroundWork) stop() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.stopped = true
	for cancel := range b.cancels {
		(*cancel)()
	}
	b.mu.Unlock()
	b.wg.Wait()
}

type resumeProbe struct {
	chatID, sessionID string
	at                time.Time
}

func newSessionBook() *sessionBook {
	return &sessionBook{
		chats:       map[string]domain.AgentSession{},
		causes:      map[string]string{},
		probes:      map[string]resumeProbe{},
		quarantined: map[string]map[string]struct{}{},
	}
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

// hasCause reports whether Crowbar itself is ending runnerID.
func (b *sessionBook) hasCause(runnerID string) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.causes[runnerID]
	return ok
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
	delete(b.quarantined, chatID)
}

func (b *sessionBook) probe(runnerID string, p resumeProbe) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probes[runnerID] = p
}

// confirm drops runnerID's probe: its CLI announced a session, so the resume
// was accepted.
func (b *sessionBook) confirm(runnerID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.probes, runnerID)
}

// failedProbe consumes runnerID's probe and, when it is still unconfirmed and
// the runner died within window of launching, quarantines the session.
func (b *sessionBook) failedProbe(runnerID string, window time.Duration) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.probes[runnerID]
	delete(b.probes, runnerID)
	if !ok || time.Since(p.at) > window {
		return false
	}
	if b.quarantined[p.chatID] == nil {
		b.quarantined[p.chatID] = map[string]struct{}{}
	}
	b.quarantined[p.chatID][p.sessionID] = struct{}{}
	return true
}

func (b *sessionBook) isQuarantined(chatID, sessionID string) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.quarantined[chatID][sessionID]
	return ok
}

// ConfirmLaunch implements turn.Runners: a CLI that reports anything took
// the session it was launched on, so its exit can never read as a refusal.
func (rs *Runners) ConfirmLaunch(runnerID string) { rs.sessions.confirm(runnerID) }

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
func (rs *Runners) noteExit(ctx context.Context, chatID, runnerID string, refused bool) {
	reason := rs.sessions.takeCause(runnerID)
	if refused {
		reason = domain.AgentExitResumeFailed
	}
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

// noteSpawnFailure records spawn_failed on a chat a spawn left with no runner.
// A CLI that died during startup already recorded why through its own exit, and
// a preempted spawn is the preempting Stop's to record.
func (rs *Runners) noteSpawnFailure(ctx context.Context, chatID string, err *error) {
	if *err == nil || errors.Is(*err, ErrProviderExitedDuringStartup) || errors.Is(*err, context.Canceled) {
		return
	}
	if _, liveErr := rs.runnerStore.LiveRunnerForChat(ctx, chatID); liveErr == nil {
		return
	}
	rs.noteChatExit(ctx, chatID, domain.AgentExitSpawnFailed)
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
