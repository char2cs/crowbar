package terminal_test

// Lifecycle invariants (stabilization spec §7-B, B1–B3).
//
// These are the rules the session state machine guarantees, stated as tests over the
// public Engine surface and driven by sequences of operations rather than one hand-picked
// interleaving each. They exist because the engine's history is a list of fixes for
// interleavings nobody wrote down (resurrection after self-exit, a double reap, an evict
// that never said "ended"); a randomized sequence finds the next one before a user does.

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// endedLedger counts OnSessionEnded and onExit deliveries per session id.
type endedLedger struct {
	mu     sync.Mutex
	ended  map[string]int
	exits  map[string]int
	signal chan string // every ended id, in order (buffered generously)
}

func newEndedLedger() *endedLedger {
	return &endedLedger{
		ended:  make(map[string]int),
		exits:  make(map[string]int),
		signal: make(chan string, 4096),
	}
}

func (l *endedLedger) onEnded(_ context.Context, _, sid string, _ int) {
	l.mu.Lock()
	l.ended[sid]++
	l.mu.Unlock()
	l.signal <- sid
}

func (l *endedLedger) onExit(sid *string) func() {
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.exits[*sid]++
	}
}

func (l *endedLedger) counts(sid string) (ended, exits int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ended[sid], l.exits[sid]
}

// waitEnded blocks until sid's ended event has been delivered.
func (l *endedLedger) waitEnded(t *testing.T, sid string) {
	t.Helper()
	if n, _ := l.counts(sid); n > 0 {
		return
	}
	timeout := time.After(10 * time.Second)
	for {
		select {
		case got := <-l.signal:
			if got == sid {
				return
			}
		case <-timeout:
			t.Fatalf("session %s never reported ended", sid)
		}
	}
}

// closingConn is a WSConn whose client "leaves" the moment it sees its first frame (the
// attach snapshot) — the shortest possible attach, which is exactly the one that races
// every other lifecycle operation.
type closingConn struct {
	once   sync.Once
	closed chan struct{}
}

func newClosingConn() *closingConn { return &closingConn{closed: make(chan struct{})} }

func (c *closingConn) WriteMessage(int, []byte) error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *closingConn) ReadMessage() (int, []byte, error) {
	<-c.closed
	return 0, nil, io.EOF
}

func (c *closingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *closingConn) SetWriteDeadline(time.Time) error { return nil }

// TestInvariant_B2_NoResurrectionAfterSelfExit pins P0-5: an Attach that lands between a
// shell's exit and its reap must never spawn a replacement under the same id. Before the
// explicit state machine, a dead-but-still-registered session read as "a placeholder" and
// was "restored" — a fresh shell under the old id — and the reaper, seeing a different
// session registered, then skipped the ended event entirely.
func TestInvariant_B2_NoResurrectionAfterSelfExit(t *testing.T) {
	pinShell(t)
	for round := 0; round < 15; round++ {
		eng := terminal.New()
		terminal.StopMaintenanceForTest(eng)
		ledger := newEndedLedger()
		eng.OnSessionEnded(ledger.onEnded)
		store := newFakeMetaStore(t)
		eng.SetMetaStore(store)

		sid := newReadyShell(t, eng, "chat-b2", store.dir)

		stop := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < 4; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					_ = eng.Attach(context.Background(), sid, newClosingConn())
				}
			}()
		}
		require.NoError(t, eng.Write(context.Background(), sid, []byte("exit\n")))
		ledger.waitEnded(t, sid)
		close(stop)
		wg.Wait()

		require.False(t, eng.SessionExists(context.Background(), sid),
			"round %d: an exited session must never be registered again", round)
		require.False(t, eng.SessionLive(context.Background(), sid),
			"round %d: nothing may run under an exited session's id", round)
		require.Error(t, eng.Attach(context.Background(), sid, newClosingConn()),
			"round %d: attaching to an exited session must fail, not restore", round)
		ended, _ := ledger.counts(sid)
		require.Equal(t, 1, ended, "round %d: exactly one ended event", round)
		require.False(t, store.hasLiveRow(sid), "round %d: no meta row survives exit", round)
		require.False(t, bufExists(store.dir, sid), "round %d: no .buf survives exit", round)
		eng.Shutdown()
	}
}

// TestInvariant_B1_ExactlyOnceUnderInterleaving pins B1: every command session reaching done
// yields exactly one ended event and exactly one onExit, whatever mix of Kill,
// TerminateGraceful, self-exit, Attach and Shutdown gets there first.
func TestInvariant_B1_ExactlyOnceUnderInterleaving(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	seed := rng.Int63()
	t.Logf("seed %d", seed)
	rng = rand.New(rand.NewSource(seed))

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ledger := newEndedLedger()
	eng.OnSessionEnded(ledger.onEnded)
	eng.SetMetaStore(newFakeMetaStore(t))
	restore := terminal.SetGracefulTerminateGraceForTest(eng, 20*time.Millisecond)
	defer restore()

	const n = 24
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		sleep := fmt.Sprintf("0.0%d", rng.Intn(9)+1)
		idx := i
		id, err := eng.CreateCommand(context.Background(), "chat-b1", t.TempDir(),
			[]string{"/bin/sh", "-c", "sleep " + sleep + "; exit 7"}, nil, ledger.onExit(&ids[idx]))
		require.NoError(t, err)
		ledger.mu.Lock()
		ids[i] = id
		ledger.mu.Unlock()

		op := rng.Intn(4)
		delay := time.Duration(rng.Intn(60)) * time.Millisecond
		wg.Add(1)
		go func(id string, op int, delay time.Duration) {
			defer wg.Done()
			time.Sleep(delay)
			switch op {
			case 0:
				_ = eng.Kill(context.Background(), id)
			case 1:
				_ = eng.TerminateGraceful(context.Background(), id)
			case 2:
				for j := 0; j < 5; j++ {
					_ = eng.Attach(context.Background(), id, newClosingConn())
				}
			case 3:
				// self-exit only
			}
		}(id, op, delay)
	}
	wg.Wait()
	eng.Shutdown() // joins every reaper: every owed callback has RUN when this returns

	for _, id := range ids {
		ended, exits := ledger.counts(id)
		require.Equal(t, 1, ended, "session %s: exactly one ended event", id)
		require.Equal(t, 1, exits, "session %s: exactly one onExit", id)
		require.False(t, eng.SessionExists(context.Background(), id))
	}
}

// TestInvariant_B3_RegistryAndDiskAgreeUnderRandomOps pins B3: after every engine operation
// the registry, the .buf file and the meta row tell the same story about each session.
//
//   - suspended  → a .buf and a "suspended" meta row exist (that is what restore reads);
//   - live       → any meta row says it is live (never "suspended");
//   - gone       → neither a .buf nor a meta row survives, and ended fired exactly once.
func TestInvariant_B3_RegistryAndDiskAgreeUnderRandomOps(t *testing.T) {
	pinShell(t)
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewSource(seed))

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ledger := newEndedLedger()
	eng.OnSessionEnded(ledger.onEnded)
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	ctx := context.Background()

	var known []string
	gone := make(map[string]bool)

	pickLive := func() (string, bool) {
		var cands []string
		for _, id := range known {
			if !gone[id] {
				cands = append(cands, id)
			}
		}
		if len(cands) == 0 {
			return "", false
		}
		return cands[rng.Intn(len(cands))], true
	}

	check := func(step int, op string) {
		t.Helper()
		for _, id := range known {
			state, ok := eng.StateOf(id)
			store.mu.Lock()
			row, hasRow := store.rows[id]
			store.mu.Unlock()
			hasBuf := bufExists(store.dir, id)
			switch {
			case !ok:
				require.True(t, gone[id], "step %d (%s): %s vanished without the test ending it", step, op, id)
				require.False(t, hasRow, "step %d (%s): gone session %s kept its meta row", step, op, id)
				require.False(t, hasBuf, "step %d (%s): gone session %s kept its .buf", step, op, id)
				ended, _ := ledger.counts(id)
				require.Equal(t, 1, ended, "step %d (%s): gone session %s: ended count", step, op, id)
			case state == "suspended":
				require.True(t, hasRow && row.State == "suspended",
					"step %d (%s): suspended %s needs a suspended meta row (have %v %q)", step, op, id, hasRow, row.State)
				require.True(t, hasBuf, "step %d (%s): suspended %s needs its .buf", step, op, id)
			default:
				require.False(t, gone[id], "step %d (%s): %s is %s after the test ended it", step, op, id, state)
				if hasRow {
					require.NotEqual(t, "suspended", row.State,
						"step %d (%s): live %s has a suspended meta row", step, op, id)
				}
			}
		}
	}

	for step := 0; step < 40; step++ {
		op := rng.Intn(7)
		name := "noop"
		switch op {
		case 0: // create
			if len(known)-len(gone) >= 4 {
				continue
			}
			name = "create"
			known = append(known, newReadyShell(t, eng, "chat-b3", store.dir))
		case 1: // suspend (idle-gated; a fresh prompt is idle)
			id, ok := pickLive()
			if !ok {
				continue
			}
			name = "suspend"
			require.NoError(t, eng.Suspend(ctx, id))
		case 2: // short attach (restores a suspended session, then detaches)
			id, ok := pickLive()
			if !ok {
				continue
			}
			name = "attach"
			require.NoError(t, eng.Attach(ctx, id, newClosingConn()))
			if eng.SessionLive(ctx, id) {
				terminal.WaitPromptForTest(t, eng, id)
			}
		case 3: // kill
			id, ok := pickLive()
			if !ok {
				continue
			}
			name = "kill"
			require.NoError(t, eng.Kill(ctx, id))
			gone[id] = true
		case 4: // self-exit (only a live shell can exit)
			id, ok := pickLive()
			if !ok || !eng.SessionLive(ctx, id) {
				continue
			}
			name = "exit"
			require.NoError(t, eng.Write(ctx, id, []byte("exit\n")))
			ledger.waitEnded(t, id)
			gone[id] = true
		case 5: // maintenance flush
			name = "flush"
			terminal.RunMaintenanceOnceForTest(eng, ctx)
		case 6: // over-ceiling sweep: suspends idle shells, then evicts suspended ones
			name = "evict"
			restoreCeil := terminal.SetMaxTotalSessionsForTest(eng, 1)
			terminal.RunMaintenanceOnceForTest(eng, ctx)
			restoreCeil()
			for _, id := range known {
				if _, ok := eng.StateOf(id); !ok && !gone[id] {
					gone[id] = true // evicted: must have reported ended (checked below)
				}
			}
		}
		check(step, name)
	}
}
