package terminal

import (
	"context"
	"fmt"
	"time"
)

func impl(eng Engine) *terminalEngine {
	e, ok := eng.(*terminalEngine)
	if !ok {
		panic(fmt.Sprintf("terminal: test helper given %T, not the terminal engine", eng))
	}
	return e
}

func RunMaintenanceOnceForTest(eng Engine, ctx context.Context) {
	impl(eng).runMaintenanceOnce(ctx)
}

func setCfg(eng Engine, fn func(*config)) (restore func()) {
	e := impl(eng)
	old := e.cfg
	fn(&e.cfg)
	return func() { e.cfg = old }
}

func SetSoftLimitPerChatForTest(eng Engine, n int) (restore func()) {
	return setCfg(eng, func(c *config) { c.softLimitPerChat = n })
}

func SetMaxTotalSessionsForTest(eng Engine, n int) (restore func()) {
	return setCfg(eng, func(c *config) { c.maxTotalSessions = n })
}

func SetMaxTotalModelBytesForTest(eng Engine, n int64) (restore func()) {
	return setCfg(eng, func(c *config) { c.maxTotalModelBytes = n })
}

func SetGracefulTerminateGraceForTest(eng Engine, d time.Duration) (restore func()) {
	return setCfg(eng, func(c *config) { c.terminateGrace = d })
}

func NewWithTickForTest(d time.Duration) Engine {
	cfg := defaultConfig()
	cfg.maintenanceTick = d
	return newEngine(cfg)
}

func NewWithWriteWaitForTest(d time.Duration) Engine {
	cfg := defaultConfig()
	cfg.writeWait = d
	return newEngine(cfg)
}

func SetLastActiveForTest(eng Engine, id string, t time.Time) {
	ent, ok := impl(eng).lookup(id)
	if !ok {
		return
	}
	ent.mu.Lock()
	ent.lastActive = t
	ent.mu.Unlock()
}

func IsIdleForTest(eng Engine, id string) bool {
	s := impl(eng).liveSession(id)
	return s != nil && s.IsIdle()
}

func PumpNotifyForTest(eng Engine, id string) <-chan struct{} {
	s := impl(eng).liveSession(id)
	if s == nil {
		return nil
	}
	return s.PumpNotifyForTest()
}

func SerializedForTest(eng Engine, id string) []byte {
	s := impl(eng).liveSession(id)
	if s == nil {
		return nil
	}
	return s.SerializedForTest()
}

func SessionDoneForTest(eng Engine, id string) <-chan struct{} {
	s := impl(eng).liveSession(id)
	if s == nil {
		return nil
	}
	return s.Done()
}

func StopMaintenanceForTest(eng Engine) {
	te := impl(eng)
	te.stopOnce.Do(func() { close(te.stop) })
	<-te.maintDone
}

func BeginDrainForTest(eng Engine) {
	_ = impl(eng).reaps.drain()
}

func DefaultLocaleForTest(
	base []string,
	goos string,
) string {
	return defaultLocale(base, goos)
}

func ParseOutputFrame(msg []byte) (payload []byte, snapshot, ok bool) {
	if len(msg) == 0 || msg[0] > FrameSnapshot {
		return nil, false, false
	}
	return msg[1:], msg[0] == FrameSnapshot, true
}
