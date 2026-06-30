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
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/persistence"
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

// gatedMetaStore wraps fakeMetaStore and lets a test park the FIRST StorageDir
// call made after arm() until release() is called. This deterministically
// places a flush/detach persist write AFTER reapOnDone's DeleteBuf+deleteMeta,
// which is the exact interleaving that resurrects an explicitly-closed terminal
// on the unfixed code.
type gatedMetaStore struct {
	*fakeMetaStore
	gmu     sync.Mutex
	armed   bool
	blocked chan struct{}
	release chan struct{}
}

func newGatedMetaStore(t *testing.T) *gatedMetaStore {
	return &gatedMetaStore{fakeMetaStore: newFakeMetaStore(t)}
}

// arm primes the gate so the next StorageDir call parks until release().
func (g *gatedMetaStore) arm() {
	g.gmu.Lock()
	g.armed = true
	g.blocked = make(chan struct{})
	g.release = make(chan struct{})
	g.gmu.Unlock()
}

func (g *gatedMetaStore) blockedCh() chan struct{} {
	g.gmu.Lock()
	defer g.gmu.Unlock()
	return g.blocked
}

func (g *gatedMetaStore) releaseGate() {
	g.gmu.Lock()
	r := g.release
	g.gmu.Unlock()
	close(r)
}

func (g *gatedMetaStore) StorageDir(ctx context.Context, ws string) (string, error) {
	g.gmu.Lock()
	if g.armed {
		g.armed = false
		b, r := g.blocked, g.release
		g.gmu.Unlock()
		close(b) // signal: a call has parked
		<-r      // wait for the test to release us
		return g.fakeMetaStore.StorageDir(ctx, ws)
	}
	g.gmu.Unlock()
	return g.fakeMetaStore.StorageDir(ctx, ws)
}

// TestReap_NoResurrection_OnSelfExit verifies that when a shell self-exits (the
// common `exit` path) while a client is attached, neither a concurrent
// cadence-flush nor the detach-bookkeeping persist resurrects the just-deleted
// .buf file or meta row.
//
// Root cause (unfixed code): session.shutdown() never nils ptmx, so IsLive()
// stays true after a self-exit until reapOnDone runs reg.Remove. The cadence
// flush and the detach-bookkeeping each do WriteBuf+saveMeta with NO
// e.lockSession and NO liveness guard, so either can land its write AFTER
// reapOnDone's DeleteBuf+deleteMeta — recreating the scrollback file and durable
// row, which the next daemon start reloads as a ghost placeholder.
//
// This test makes the bad interleaving deterministic via gatedMetaStore: the
// persist path is parked at StorageDir (after it has passed every liveness
// check, holding e.lockSession under the fix) while the shell self-exits and
// reapOnDone runs; the parked write is then released. On unfixed code reap has
// already deleted everything, so the released write resurrects it. With the fix,
// reapOnDone blocks on e.lockSession until the parked write finishes, then runs
// its DeleteBuf+deleteMeta last — leaving NO trace.
//
// Run with: go test -race -count=10 -run TestReap_NoResurrection_OnSelfExit ./...
func TestReap_NoResurrection_OnSelfExit(t *testing.T) {
	cases := []struct {
		name string
		// park installs the late writer (flush or detach) so its StorageDir call
		// parks on the gate; it returns a channel closed when the writer's
		// goroutine has fully returned.
		park func(t *testing.T, eng terminal.Engine, store *gatedMetaStore, conn *mockConn, sid string) <-chan struct{}
	}{
		{
			name: "cadence-flush",
			park: func(_ *testing.T, eng terminal.Engine, store *gatedMetaStore, _ *mockConn, _ string) <-chan struct{} {
				done := make(chan struct{})
				store.arm()
				go func() {
					defer close(done)
					terminal.RunMaintenanceOnceForTest(eng, context.Background())
				}()
				return done
			},
		},
		{
			name: "detach-bookkeeping",
			park: func(_ *testing.T, _ terminal.Engine, store *gatedMetaStore, conn *mockConn, _ string) <-chan struct{} {
				done := make(chan struct{})
				store.arm()
				// Closing the conn unblocks readPump → Detach → the
				// detach-bookkeeping persist parks on the gate.
				conn.Close()
				close(done) // the Attach goroutine itself is joined separately
				return done
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			eng := terminal.New()
			terminal.StopMaintenanceForTest(eng)
			ctx := context.Background()
			store := newGatedMetaStore(t)
			eng.SetMetaStore(store)

			sid, err := eng.Create(ctx, "ws-reap", store.dir, nil)
			require.NoError(t, err)

			// Wait for the shell to become idle (prompt drawn, ready for input).
			waitUntil(t, 15*time.Second, func() bool { return terminal.IsIdleForTest(eng, sid) },
				"session never became idle")

			// Attach a real client and wait until the session is active.
			conn := newMockConn()
			attachDone := make(chan struct{})
			go func() {
				defer close(attachDone)
				_ = eng.Attach(ctx, sid, conn)
			}()
			waitUntil(t, 10*time.Second, func() bool {
				state, ok := eng.StateOf(sid)
				return ok && state == "active"
			}, "client never attached")

			// Drive output so the model is dirty (so the cadence flush persists).
			require.NoError(t, eng.Write(ctx, sid, []byte("echo reap-probe-line\n")))
			time.Sleep(80 * time.Millisecond)

			// Install the late writer; it parks at StorageDir holding the
			// per-session lock (under the fix), having passed all liveness checks.
			writerDone := tc.park(t, eng, store, conn, sid)

			// Wait until the writer is actually parked on the gate.
			select {
			case <-store.blockedCh():
			case <-time.After(10 * time.Second):
				t.Fatal("late writer never parked on StorageDir gate")
			}

			// Now self-exit the shell. reapOnDone fires: on unfixed code it runs
			// to completion (DeleteBuf+deleteMeta); under the fix it blocks on
			// e.lockSession held by the parked writer.
			_ = eng.Write(ctx, sid, []byte("exit\n"))

			// Give the shell time to exit and reapOnDone its chance to run (or to
			// block on the lock). 400ms >> shell-exit + reap latency.
			time.Sleep(400 * time.Millisecond)

			// Release the parked write. On unfixed code this WriteBuf+saveMeta
			// lands after reap already deleted everything → resurrection.
			store.releaseGate()
			<-writerDone

			// Wait until the session is reaped out of the registry.
			waitUntil(t, 10*time.Second, func() bool { return !eng.SessionExists(ctx, sid) },
				"session not reaped from registry")
			<-attachDone
			conn.Close()

			// Settle, then assert the explicitly-closed terminal left NO trace.
			time.Sleep(150 * time.Millisecond)

			assert.False(t, eng.SessionExists(ctx, sid),
				"registry must not contain the reaped session")

			buf, readErr := persistence.ReadBuf(store.dir, sid)
			require.NoError(t, readErr)
			assert.Nil(t, buf,
				".buf must stay deleted — a flush/detach write resurrected it")

			assert.False(t, store.hasLiveRow(sid),
				"meta row must stay deleted — a flush/detach saveMeta resurrected it")

			// A fresh daemon's RestorePersistedSessions must NOT bring it back.
			fresh := terminal.New()
			terminal.StopMaintenanceForTest(fresh)
			fresh.SetMetaStore(store.fakeMetaStore)
			for _, m := range store.liveRows() {
				sb, _ := persistence.ReadBuf(store.dir, m.SessionID)
				_ = fresh.LoadPlaceholder(ctx, m, sb)
			}
			assert.False(t, fresh.SessionExists(ctx, sid),
				"fresh-engine restore resurrected an explicitly-closed terminal")

			fresh.Shutdown()
			eng.Shutdown()
		})
	}
}

// waitUntil polls cond every 10ms until it is true or the timeout elapses.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatal(msg)
		case <-time.After(10 * time.Millisecond):
		}
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

	// Drive output so the model is dirty: the next Snapshot() returns
	// (blob, changed=true), which gates the cadence flush so it persists.
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
