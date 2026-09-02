package terminal_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/persistence"
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
	pinShell(t) // the restored shell must announce its readiness with a known prompt

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
		SessionID: sid,
		ChatID:    "chat-concurrent",
		CWD:       store.dir,
		Shell:     "/bin/sh",
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

	// Block until the restored shell has printed its prompt. The prompt is proof the
	// placeholder has become a live PTY with a running shell — the state polling StateOf
	// every 10 ms was trying to detect, but observed at the source rather than inferred from
	// a registry field. Waiting on conns[0] specifically is enough: the restore is a single
	// shared event, and the per-conn assertion below is what proves the rest arrived.
	waitForMsg(t, conns[0], func(d string) bool { return containsStr(d, shellPrompt) })
	state, ok := eng.StateOf(sid)
	require.True(t, ok)
	require.NotEqual(t, "suspended", state, "the session must be live once its shell has prompted")

	// Write the probe to the live session. Only goroutines attached to the live
	// session will receive it — goroutines frozen on the placeholder will not.
	require.NoError(t, eng.Write(ctx, sid, []byte("echo "+liveProbe+"\n")))

	// Block until EVERY goroutine's connection has received the live probe. Each wait ends
	// on that conn's own fan-out signal, which is precisely the observable the test is about
	// (a goroutine frozen on the placeholder never receives it), so a conn that never gets
	// the probe hangs here and is reported by the -timeout backstop — the very failure the
	// test exists to catch.
	for _, c := range conns {
		waitForMsg(t, c, func(d string) bool { return containsStr(d, liveProbe) })
	}
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
	pinShell(t)

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

		// The shell must be at its prompt (hence idle) for Suspend to be eligible — that is
		// the precondition of the race under test, so it is established by observation.
		sid := newReadyShell(t, eng, "chat-ks", store.dir)

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

		// reapOnDone fires OnSessionEnded from its own goroutine. Rather than poll for that
		// fire, JOIN the goroutine: Shutdown drains every outstanding reaper before it
		// returns, so once it has, the ended count is final and cannot change again.
		//
		// That makes the assertion exact in BOTH directions, which an Eventually could never
		// be: it proves the callback fired, and — because no reaper is still running — that
		// it fired exactly ONCE. The old Eventually returned the instant the count hit 1 and
		// would have been blind to a second, duplicate fire arriving a moment later.
		eng.Shutdown()

		assert.Equal(t, int64(1), atomic.LoadInt64(&endedCount),
			"round %d: OnSessionEnded must fire exactly once", round)
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

func (g *gatedMetaStore) StorageDir(ctx context.Context, chatID string) (string, error) {
	g.gmu.Lock()
	if g.armed {
		g.armed = false
		b, r := g.blocked, g.release
		g.gmu.Unlock()
		close(b) // signal: a call has parked
		<-r      // wait for the test to release us
		return g.fakeMetaStore.StorageDir(ctx, chatID)
	}
	g.gmu.Unlock()
	return g.fakeMetaStore.StorageDir(ctx, chatID)
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
			pinShell(t)

			eng := terminal.New()
			terminal.StopMaintenanceForTest(eng)
			ctx := context.Background()
			store := newGatedMetaStore(t)
			eng.SetMetaStore(store)

			// reapOnDone fires OnSessionEnded LAST — after reg.Remove, DeleteBuf and
			// deleteMeta. So this callback is the exact "fully reaped, everything deleted"
			// signal, and blocking on it below replaces both a registry poll and an
			// Eventually over the filesystem.
			reaped := make(chan struct{})
			eng.OnSessionEnded(func(_ context.Context, _, _ string, _ int) { close(reaped) })

			// Block until the shell is at its prompt (drawn, idle, ready for input).
			sid := newReadyShell(t, eng, "chat-reap", store.dir)

			// Attach a real client. The first frame the conn receives is Attach's snapshot
			// keyframe, so receiving anything at all proves the client is attached and
			// streaming — the observable that polling StateOf=="active" was approximating.
			conn := newMockConn()
			attachDone := make(chan struct{})
			go func() {
				defer close(attachDone)
				_ = eng.Attach(ctx, sid, conn)
			}()
			waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })
			state, ok := eng.StateOf(sid)
			require.True(t, ok)
			require.Equal(t, "active", state, "a session with a streaming client is active")

			// Drive output so the model is dirty (so the cadence flush has something to
			// persist). runShell returns only once the command's output AND the following
			// prompt are through the pump — i.e. the model is dirty AND the shell is back at
			// its prompt, both established by observation rather than by two separate polls.
			runShell(t, eng, sid, "echo reap-probe-line")

			// Capture the death channel BEFORE the exit: it is the session's own signal, and
			// reapOnDone will shortly remove the session from the registry, after which it
			// could no longer be looked up.
			shellDead := terminal.SessionDoneForTest(eng, sid)
			require.NotNil(t, shellDead)

			// Install the late writer; it parks at StorageDir holding the
			// per-session lock (under the fix), having passed all liveness checks.
			writerDone := tc.park(t, eng, store, conn, sid)

			// Block until the writer is actually parked on the gate. blockedCh is a real
			// signal from the gate itself; it needed no 10-second deadline beside it.
			<-store.blockedCh()

			// Now self-exit the shell. reapOnDone fires: on unfixed code it runs
			// to completion (DeleteBuf+deleteMeta); under the fix it blocks on
			// e.lockSession held by the parked writer.
			_ = eng.Write(ctx, sid, []byte("exit\n"))

			// Block until the shell has actually exited. The session's done channel closes
			// when the PTY dies — that IS the event, so we wait on it directly instead of
			// polling IsIdle and reading its TIOCGPGRP error as a proxy for death. This is
			// the point at which reapOnDone has unblocked from <-s.Done() and is either
			// racing to clean up (unfixed) or blocked on the per-session lock held by the
			// parked writer (the fix).
			<-shellDead

			// Release the parked write. On unfixed code this WriteBuf+saveMeta
			// lands after reap already deleted everything → resurrection.
			store.releaseGate()
			<-writerDone

			// Block until reapOnDone has completed its cleanup. OnSessionEnded is its final
			// act, so when this returns the registry entry is gone and DeleteBuf/deleteMeta
			// have already run — the two facts asserted below.
			<-reaped
			<-attachDone
			conn.Close()

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

// waitUntil — a generic "poll cond every 10 ms until a timeout" helper — is deliberately
// gone. Every one of its callers was waiting for a specific event that the engine already
// signals: the shell reaching its prompt (waitPrompt), a client receiving a frame
// (waitForMsg), the PTY dying (SessionDoneForTest), a session being fully reaped
// (OnSessionEnded), a gate being reached (gatedMetaStore.blockedCh). Each now blocks on
// that signal, so none of them can wake early on a stale read or expire under load.

// TestFlush_Serialized_NewestWins verifies that two concurrent cadence-flush
// triggers via RunMaintenanceOnceForTest do not corrupt the scrollback and that
// the operation completes without data races (checked by -race). With FIX 2,
// every flush site holds s.FlushMu() across Snapshot+WriteBuf so concurrent
// callers are serialized and the newest snapshot always wins.
func TestFlush_Serialized_NewestWins(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := newReadyShell(t, eng, "chat-flush", store.dir)

	// Drive output so the model is dirty: the next Snapshot() returns (blob, changed=true),
	// which gates the cadence flush so it persists. runShell blocks until that output has
	// actually been through the pump, so the racing sweeps below are guaranteed to find a
	// dirty session — rather than merely being likely to.
	runShell(t, eng, sid, "echo flush-test-output")

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
