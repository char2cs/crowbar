package v0

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

const leakTolerance = 10

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
