package terminal

import (
	"context"
	"time"
)

// RunMaintenanceOnceForTest exposes runMaintenanceOnce for unit tests so they
// can drive the maintenance sweep directly without waiting for the ticker.
func RunMaintenanceOnceForTest(eng Engine, ctx context.Context) {
	eng.(*terminalEngine).runMaintenanceOnce(ctx)
}

// setCfg applies fn to eng's config and returns a function restoring the old one. Tests
// must have stopped the maintenance goroutine first (StopMaintenanceForTest), which is the
// only other reader.
func setCfg(eng Engine, fn func(*config)) (restore func()) {
	e := eng.(*terminalEngine)
	old := e.cfg
	fn(&e.cfg)
	return func() { e.cfg = old }
}

// SetSoftLimitPerChatForTest overrides eng's per-chat detached-session soft limit.
func SetSoftLimitPerChatForTest(eng Engine, n int) (restore func()) {
	return setCfg(eng, func(c *config) { c.softLimitPerChat = n })
}

// SetMaxTotalSessionsForTest overrides eng's global session-count ceiling.
func SetMaxTotalSessionsForTest(eng Engine, n int) (restore func()) {
	return setCfg(eng, func(c *config) { c.maxTotalSessions = n })
}

// SetMaxTotalModelBytesForTest overrides eng's global model-bytes ceiling.
func SetMaxTotalModelBytesForTest(eng Engine, n int64) (restore func()) {
	return setCfg(eng, func(c *config) { c.maxTotalModelBytes = n })
}

// SetGracefulTerminateGraceForTest overrides eng's TerminateGraceful grace window, so a
// test can exercise the fallback-to-hard-kill path without a multi-second sleep.
func SetGracefulTerminateGraceForTest(eng Engine, d time.Duration) (restore func()) {
	return setCfg(eng, func(c *config) { c.terminateGrace = d })
}

// NewWithTickForTest builds an engine whose maintenance sweep ticks every d.
func NewWithTickForTest(d time.Duration) Engine {
	cfg := defaultConfig()
	cfg.maintenanceTick = d
	return newEngine(cfg)
}

// NewWithWriteWaitForTest builds an engine whose WebSocket writes time out after d.
func NewWithWriteWaitForTest(d time.Duration) Engine {
	cfg := defaultConfig()
	cfg.writeWait = d
	return newEngine(cfg)
}

// SetLastActiveForTest sets a session's last-active time, so tests control LRU ordering
// without real delays.
func SetLastActiveForTest(eng Engine, id string, t time.Time) {
	ent, ok := eng.(*terminalEngine).lookup(id)
	if !ok {
		return
	}
	ent.mu.Lock()
	ent.lastActive = t
	ent.mu.Unlock()
}

// IsIdleForTest reports whether the live session with the given ID is idle.
func IsIdleForTest(eng Engine, id string) bool {
	s := eng.(*terminalEngine).liveSession(id)
	return s != nil && s.IsIdle()
}

// PumpNotifyForTest returns the live session's pump-progress signal (see
// session.PumpNotifyForTest). Returns nil if the session is not live — a caller blocking on
// a nil channel blocks forever, which `go test -timeout` reports as the hang it is.
func PumpNotifyForTest(eng Engine, id string) <-chan struct{} {
	s := eng.(*terminalEngine).liveSession(id)
	if s == nil {
		return nil
	}
	return s.PumpNotifyForTest()
}

// SerializedForTest returns the live session's current serialized screen (non-consuming).
func SerializedForTest(eng Engine, id string) []byte {
	s := eng.(*terminalEngine).liveSession(id)
	if s == nil {
		return nil
	}
	return s.SerializedForTest()
}

// SessionDoneForTest returns the live session's death channel, or nil.
func SessionDoneForTest(eng Engine, id string) <-chan struct{} {
	s := eng.(*terminalEngine).liveSession(id)
	if s == nil {
		return nil
	}
	return s.Done()
}

// StopMaintenanceForTest stops the background maintenance goroutine without killing any
// session, so a test that drives maintenance manually or changes limits races nothing.
// Shutdown remains safe afterwards.
func StopMaintenanceForTest(eng Engine) {
	te := eng.(*terminalEngine)
	te.stopOnce.Do(func() { close(te.stop) })
	<-te.maintDone
}

// BeginDrainForTest closes the engine's session-birth door WITHOUT killing or
// deregistering anything: the state Shutdown occupies from the moment it starts draining
// until its walk reaches a given session.
func BeginDrainForTest(eng Engine) {
	_ = eng.(*terminalEngine).reaps.drain()
}

// DefaultLocaleForTest exposes the internal defaultLocale decision to the
// package's external unit tests so they can assert ptyEnv's per-GOOS UTF-8
// fallback for a synthetic environment without mutating the real process
// environment. It returns the LANG value ptyEnv would inject for the given base
// environment and GOOS, or "" when a locale is already set.
func DefaultLocaleForTest(
	base []string,
	goos string,
) string {
	return defaultLocale(base, goos)
}

// ParseOutputFrame splits one binary output message into its payload and
// whether it is a snapshot; ok is false for anything that is not an output frame.
func ParseOutputFrame(msg []byte) (payload []byte, snapshot bool, ok bool) {
	if len(msg) == 0 || msg[0] > FrameSnapshot {
		return nil, false, false
	}
	return msg[1:], msg[0] == FrameSnapshot, true
}
