package terminal_test

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

type exitLedger struct {
	mu    sync.Mutex
	exits map[int]int
}

func newExitLedger() *exitLedger { return &exitLedger{exits: make(map[int]int)} }

func (l *exitLedger) onExit(slot int) func(terminal.Exit) {
	return func(terminal.Exit) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.exits[slot]++
	}
}

func (l *exitLedger) exitsOf(slot int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.exits[slot]
}

func waitGone(t *testing.T, eng terminal.Engine, store *fakeMetaStore, sid string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return !eng.SessionExists(context.Background(), sid) && !store.hasLiveRow(sid) && !bufExists(store.dir, sid)
	}, 10*time.Second, 5*time.Millisecond, "session %s never ended", sid)
}

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

func TestInvariant_B2_NoResurrectionAfterSelfExit(t *testing.T) {
	pinShell(t)
	for round := 0; round < 15; round++ {
		eng := terminal.New()
		terminal.StopMaintenanceForTest(eng)
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
		waitGone(t, eng, store, sid)
		close(stop)
		wg.Wait()

		require.False(t, eng.SessionExists(context.Background(), sid),
			"round %d: an exited session must never be registered again", round)
		require.False(t, eng.SessionLive(context.Background(), sid),
			"round %d: nothing may run under an exited session's id", round)
		require.Error(t, eng.Attach(context.Background(), sid, newClosingConn()),
			"round %d: attaching to an exited session must fail, not restore", round)
		require.False(t, store.hasLiveRow(sid), "round %d: no meta row survives exit", round)
		require.False(t, bufExists(store.dir, sid), "round %d: no .buf survives exit", round)
		eng.Shutdown()
	}
}

func TestInvariant_B1_ExactlyOnceUnderInterleaving(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	seed := rng.Int63()
	t.Logf("seed %d", seed)
	rng = rand.New(rand.NewSource(seed))

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ledger := newExitLedger()
	eng.SetMetaStore(newFakeMetaStore(t))
	restore := terminal.SetGracefulTerminateGraceForTest(eng, 20*time.Millisecond)
	defer restore()

	const n = 24
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		sleep := fmt.Sprintf("0.0%d", rng.Intn(9)+1)
		id, err := eng.CreateCommand(context.Background(), "chat-b1", t.TempDir(),
			[]string{"/bin/sh", "-c", "sleep " + sleep + "; exit 7"}, nil, ledger.onExit(i))
		require.NoError(t, err)
		ids[i] = id

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
			}
		}(id, op, delay)
	}
	wg.Wait()
	eng.Shutdown()

	for i, id := range ids {
		require.Equal(t, 1, ledger.exitsOf(i), "session %s: exactly one onExit", id)
		require.False(t, eng.SessionExists(context.Background(), id))
	}
}

func TestInvariant_B3_RegistryAndDiskAgreeUnderRandomOps(t *testing.T) {
	pinShell(t)
	seed := time.Now().UnixNano()
	t.Logf("seed %d", seed)
	rng := rand.New(rand.NewSource(seed))

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
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
		case 0:
			if len(known)-len(gone) >= 4 {
				continue
			}
			name = "create"
			known = append(known, newReadyShell(t, eng, "chat-b3", store.dir))
		case 1:
			id, ok := pickLive()
			if !ok {
				continue
			}
			name = "suspend"
			require.NoError(t, eng.Suspend(ctx, id))
		case 2:
			id, ok := pickLive()
			if !ok {
				continue
			}
			name = "attach"
			require.NoError(t, eng.Attach(ctx, id, newClosingConn()))
			if eng.SessionLive(ctx, id) {
				terminal.WaitPromptForTest(t, eng, id)
			}
		case 3:
			id, ok := pickLive()
			if !ok {
				continue
			}
			name = "kill"
			require.NoError(t, eng.Kill(ctx, id))
			gone[id] = true
		case 4:
			id, ok := pickLive()
			if !ok || !eng.SessionLive(ctx, id) {
				continue
			}
			name = "exit"
			require.NoError(t, eng.Write(ctx, id, []byte("exit\n")))
			waitGone(t, eng, store, id)
			gone[id] = true
		case 5:
			name = "flush"
			terminal.RunMaintenanceOnceForTest(eng, ctx)
		case 6:
			name = "evict"
			restoreCeil := terminal.SetMaxTotalSessionsForTest(eng, 1)
			terminal.RunMaintenanceOnceForTest(eng, ctx)
			restoreCeil()
			for _, id := range known {
				if _, ok := eng.StateOf(id); !ok && !gone[id] {
					gone[id] = true
				}
			}
		}
		check(step, name)
	}
}
