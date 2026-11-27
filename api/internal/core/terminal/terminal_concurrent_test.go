package terminal_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

func TestRestore_ConcurrentAttach_NoOrphan(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := "concurrent-restore-orphan"
	scrollback := []byte("old scrollback\r\n")

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

	waitForMsg(t, conns[0], func(d string) bool { return containsStr(d, shellPrompt) })
	state, ok := eng.StateOf(sid)
	require.True(t, ok)
	require.NotEqual(t, "suspended", state, "the session must be live once its shell has prompted")

	require.NoError(t, eng.Write(ctx, sid, []byte("echo "+liveProbe+"\n")))

	for _, c := range conns {
		waitForMsg(t, c, func(d string) bool { return containsStr(d, liveProbe) })
	}
	for _, c := range conns {
		_ = c.Close()
	}
	wg.Wait()

	for i, c := range conns {
		found := false
		for _, raw := range c.allReceived() {
			data, _, ok := terminal.ParseOutputFrame(raw)
			if !ok {
				continue
			}
			if containsStr(string(data), liveProbe) {
				found = true
				break
			}
		}
		assert.True(t, found,
			"goroutine %d did not receive live PTY output (attached to placeholder?)", i)
	}

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestKill_vs_Suspend_Serialized(t *testing.T) {
	pinShell(t)

	const rounds = 10
	for round := 0; round < rounds; round++ {
		eng := terminal.New()
		terminal.StopMaintenanceForTest(eng)
		ctx := context.Background()
		store := newFakeMetaStore(t)
		eng.SetMetaStore(store)

		sid := newReadyShell(t, eng, "chat-ks", store.dir)

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
			state, ok := eng.StateOf(sid)
			assert.True(t, ok, "round %d: session exists but StateOf returned !ok", round)
			assert.Equal(t, "suspended", state,
				"round %d: existing session after Kill/Suspend race must be suspended", round)
			require.NoError(t, eng.Kill(ctx, sid))
		}

		assert.False(t, eng.SessionExists(ctx, sid),
			"round %d: session must not exist after final cleanup", round)

		eng.Shutdown()

		assert.False(t, store.hasLiveRow(sid), "round %d: no meta row survives the kill", round)
		assert.False(t, bufExists(store.dir, sid), "round %d: no .buf survives the kill", round)
	}
}

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
		close(b)
		<-r
		return g.fakeMetaStore.StorageDir(ctx, chatID)
	}
	g.gmu.Unlock()
	return g.fakeMetaStore.StorageDir(ctx, chatID)
}

func TestReap_NoResurrection_OnSelfExit(t *testing.T) {
	cases := []struct {
		name string
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
				_ = conn.Close()
				close(done)
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

			sid := newReadyShell(t, eng, "chat-reap", store.dir)

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

			runShell(t, eng, sid, "echo reap-probe-line")

			shellDead := terminal.SessionDoneForTest(eng, sid)
			require.NotNil(t, shellDead)

			writerDone := tc.park(t, eng, store, conn, sid)

			<-store.blockedCh()

			_ = eng.Write(ctx, sid, []byte("exit\n"))

			<-shellDead

			store.releaseGate()
			<-writerDone

			waitGone(t, eng, store.fakeMetaStore, sid)
			<-attachDone
			_ = conn.Close()

			assert.False(t, eng.SessionExists(ctx, sid),
				"registry must not contain the reaped session")

			buf, readErr := readBuf(store.dir, sid)
			require.NoError(t, readErr)
			assert.Nil(t, buf,
				".buf must stay deleted — a flush/detach write resurrected it")

			assert.False(t, store.hasLiveRow(sid),
				"meta row must stay deleted — a flush/detach saveMeta resurrected it")

			fresh := terminal.New()
			terminal.StopMaintenanceForTest(fresh)
			fresh.SetMetaStore(store.fakeMetaStore)
			for _, m := range store.liveRows() {
				sb, _ := readBuf(store.dir, m.SessionID)
				_ = fresh.LoadPlaceholder(ctx, m, sb)
			}
			assert.False(t, fresh.SessionExists(ctx, sid),
				"fresh-engine restore resurrected an explicitly-closed terminal")

			fresh.Shutdown()
			eng.Shutdown()
		})
	}
}

func TestFlush_Serialized_NewestWins(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := newReadyShell(t, eng, "chat-flush", store.dir)

	runShell(t, eng, sid, "echo flush-test-output")

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

	assert.True(t, bufExists(store.dir, sid),
		".buf must exist after concurrent maintenance flushes")

	require.NoError(t, eng.Kill(ctx, sid))
}

func readBuf(dir, sessionID string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dir, sessionID+".buf"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}
