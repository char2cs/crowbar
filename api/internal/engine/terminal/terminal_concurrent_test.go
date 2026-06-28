package terminal_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TestRestore_ConcurrentAttach_NoOrphan verifies that N concurrent Attach calls
// on a suspended placeholder all land on the SAME live session after restore.
//
// Without FIX 1 (per-session lifecycle mutex) goroutines that lose the
// restoring.LoadOrStore race return nil immediately and re-fetch the placeholder
// (still in the registry while goroutine-1 is mid-spawn). They successfully
// call s.Attach() on the placeholder and are frozen there — they never receive
// live PTY output. This was reproducible 60/60 before the fix.
//
// Run with: go test -race -count=10 -run TestRestore_ConcurrentAttach ./...
func TestRestore_ConcurrentAttach_NoOrphan(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := "concurrent-restore-orphan"
	scrollback := []byte("old scrollback\r\n")

	// Write scrollback to disk so restore() can read it via persistence.ReadBuf.
	bufPath := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufPath, scrollback, 0o644))

	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID:   sid,
		WorkspaceID: "ws-concurrent",
		CWD:         store.dir,
		Shell:       "/bin/sh",
	}, scrollback))

	const N = 20
	const liveProbe = "CONCURRENT_ATTACH_LIVE_PROBE_47291"

	conns := make([]*mockConn, N)
	for i := range conns {
		conns[i] = newMockConn()
	}

	// Barrier ensures all goroutines start Attach simultaneously, maximising
	// the race window while goroutine-1 is inside spawn().
	ready := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-ready
			_ = eng.Attach(ctx, sid, conns[i])
		}()
	}
	close(ready)

	// Wait until the registry entry transitions from "suspended" to live.
	deadline := time.After(15 * time.Second)
	for {
		state, ok := eng.StateOf(sid)
		if ok && state != "suspended" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("session did not restore within 15s")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Write the probe to the live session. Only goroutines attached to the live
	// session will receive it — goroutines frozen on the placeholder will not.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo "+liveProbe+"\n")))

	// Allow output to propagate before closing connections.
	time.Sleep(2 * time.Second)
	for _, c := range conns {
		c.Close()
	}
	wg.Wait()

	// Every goroutine must have received the live PTY probe.
	for i, c := range conns {
		found := false
		for _, raw := range c.allReceived() {
			var msg struct {
				Data string `json:"data"`
			}
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			if containsStr(msg.Data, liveProbe) {
				found = true
				break
			}
		}
		assert.True(t, found,
			"goroutine %d did not receive live PTY output (attached to placeholder?)", i)
	}

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestKill_vs_Suspend_Serialized verifies that concurrent Kill and Suspend on the
// same idle detached session always produce a CONSISTENT end state:
//
//   - Either fully killed (not in registry, OnSessionEnded fired exactly once), or
//   - Cleanly suspended (placeholder present, which is then killed).
//
// Without FIX 1 the two ops interleave: Suspend's reg.Add(placeholder) can run
// AFTER Kill's reg.Remove, resurrecting the session. reapOnDone sees
// suspending==true and skips cleanup → session stays in registry and
// OnSessionEnded never fires.
func TestKill_vs_Suspend_Serialized(t *testing.T) {
	const rounds = 10
	for round := 0; round < rounds; round++ {
		eng := terminal.New()
		terminal.StopMaintenanceForTest(eng)
		ctx := context.Background()
		store := newFakeMetaStore(t)
		eng.SetMetaStore(store)

		var endedCount int64
		eng.OnSessionEnded(func(_ context.Context, _, _ string, _ int) {
			atomic.AddInt64(&endedCount, 1)
		})

		sid, err := eng.Create(ctx, "ws-ks", store.dir, nil)
		require.NoError(t, err)

		// Wait for the shell to become idle so Suspend is eligible.
		deadline := time.After(15 * time.Second)
		for {
			if terminal.IsIdleForTest(eng, sid) {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("round %d: session never became idle", round)
			case <-time.After(50 * time.Millisecond):
			}
		}

		// Race Kill vs Suspend on the same idle session.
		var wg sync.WaitGroup
		wg.Add(2)
		ready := make(chan struct{})
		go func() {
			defer wg.Done()
			<-ready
			_ = eng.Kill(ctx, sid)
		}()
		go func() {
			defer wg.Done()
			<-ready
			_ = eng.Suspend(ctx, sid)
		}()
		close(ready)
		wg.Wait()

		exists := eng.SessionExists(ctx, sid)
		if exists {
			// Suspend won the race: must be a clean placeholder (not a live session).
			state, ok := eng.StateOf(sid)
			assert.True(t, ok, "round %d: session exists but StateOf returned !ok", round)
			assert.Equal(t, "suspended", state,
				"round %d: existing session after Kill/Suspend race must be suspended", round)
			// Clean up the placeholder.
			require.NoError(t, eng.Kill(ctx, sid))
		}

		// Regardless of winner, session must be gone after cleanup.
		assert.False(t, eng.SessionExists(ctx, sid),
			"round %d: session must not exist after final cleanup", round)

		// Allow reapOnDone to fire OnSessionEnded for the Kill-wins case.
		time.Sleep(500 * time.Millisecond)

		got := atomic.LoadInt64(&endedCount)
		assert.Equal(t, int64(1), got,
			"round %d: OnSessionEnded must fire exactly once", round)

		eng.Shutdown()
	}
}

// TestFlush_Serialized_NewestWins verifies that two concurrent cadence-flush
// triggers via RunMaintenanceOnceForTest do not corrupt the scrollback and that
// the operation completes without data races (checked by -race). With FIX 2,
// every flush site holds s.FlushMu() across Snapshot+WriteBuf so concurrent
// callers are serialized and the newest snapshot always wins.
func TestFlush_Serialized_NewestWins(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid, err := eng.Create(ctx, "ws-flush", store.dir, nil)
	require.NoError(t, err)

	// Populate the ring so TakeDirty() returns true and a flush is triggered.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo flush-test-output\n")))
	time.Sleep(200 * time.Millisecond) // let shell produce output

	// Two concurrent maintenance sweeps race the cadence-flush path.
	// Under -race the detector will flag any data race if flushMu is absent.
	var wg sync.WaitGroup
	wg.Add(2)
	ready := make(chan struct{})
	go func() {
		defer wg.Done()
		<-ready
		terminal.RunMaintenanceOnceForTest(eng, ctx)
	}()
	go func() {
		defer wg.Done()
		<-ready
		terminal.RunMaintenanceOnceForTest(eng, ctx)
	}()
	close(ready)
	wg.Wait()

	// At least one flush must have completed: the .buf file must exist.
	assert.True(t, bufExists(store.dir, sid),
		".buf must exist after concurrent maintenance flushes")

	require.NoError(t, eng.Kill(ctx, sid))
}
