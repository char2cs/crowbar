package v0

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

// leakTolerance absorbs runtime and pooled-client goroutines (an idle HTTP/2
// keep-alive, say). One leaked app container is hundreds: 6 asynx pools of
// 8 shards x 8 workers, so the bound catches exactly that class of leak.
const leakTolerance = 10

// TestMain fails the package when its tests leave goroutines running, which
// is what let every test's asynx pools pile up behind the one that hung.
func TestMain(m *testing.M) {
	before := runtime.NumGoroutine()
	code := m.Run()
	if code == 0 && !goroutinesSettle(before+leakTolerance, 5*time.Second) {
		buf := make([]byte, 1<<20)
		fmt.Fprintf(os.Stderr, "goroutine leak: %d running after the tests, %d before\n%s\n",
			runtime.NumGoroutine(), before, buf[:runtime.Stack(buf, true)])
		code = 1
	}
	os.Exit(code)
}

// goroutinesSettle waits, bounded by within, for teardown goroutines that
// exit asynchronously after Close returns to finish.
func goroutinesSettle(limit int, within time.Duration) bool {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(within)
	for runtime.NumGoroutine() > limit {
		select {
		case <-deadline:
			return false
		case <-tick.C:
		}
	}
	return true
}
