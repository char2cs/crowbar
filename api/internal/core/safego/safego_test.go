package safego_test

import (
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

func TestGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	// A panic in the spawned goroutine must be contained, not propagated to crash
	// the test process. If it weren't recovered, the runtime would abort here.
	safego.Go("test.panic", func() {
		defer wg.Done()
		panic("boom")
	})
	wg.Wait()
}

func TestGo_RunsNormally(t *testing.T) {
	done := make(chan int, 1)
	safego.Go("test.normal", func() { done <- 42 })
	if got := <-done; got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

func TestRecoverFn_InvokesOnPanic(t *testing.T) {
	var got any
	func() {
		defer safego.RecoverFn("test.recoverfn", func(r any) { got = r })
		panic("surfaced")
	}()
	if got != "surfaced" {
		t.Fatalf("onPanic not invoked with the recovered value; got %v", got)
	}
}

func TestRecover_NoPanic_NoOp(t *testing.T) {
	// Recover on a clean path must be a no-op (no spurious onPanic / log noise).
	ran := false
	func() {
		defer safego.Recover("test.clean")
		ran = true
	}()
	if !ran {
		t.Fatal("body did not run")
	}
}
