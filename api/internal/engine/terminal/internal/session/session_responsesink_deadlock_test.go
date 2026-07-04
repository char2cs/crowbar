//go:build !windows

package session

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

// TestModelDriven_ResponseSinkNeverWedgesTeardown pins the C1 fix: the model's
// device-query reply path (spec §3.8) must not run the blocking ptmx.Write on the
// drain goroutine while s.mu is held. Before the fix, a foreground app that emits
// a burst of CPR queries (ESC[6n) without draining its own stdin fills the PTY
// input buffer, blocks the sink's ptmx.Write inside vtEmu.drainResponses, stalls
// the drain's next pipe read, blocks emu.Write inside pumpStep (which holds s.mu),
// and permanently wedges Kill — which itself needs s.mu.
//
// The fake ptmx here is the WRITE end of an os.Pipe whose read end is never
// drained: once ~64KiB of replies accumulate, ptmx.Write blocks forever. The burst
// generates far more than that. With the bounded-queue + writer-goroutine fix, the
// blocking write happens OFF the lock and the sink's non-blocking send drops once
// saturated, so pumpStep completes and Kill returns promptly. Against the old
// direct-write sink this test HANGS (Kill never returns) and fails on the deadline.
func TestModelDriven_ResponseSinkNeverWedgesTeardown(t *testing.T) {
	s := newBareSession("sid-md-sink-deadlock", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	// Fake ptmx: an os.Pipe write end with no reader. ptmx.Write blocks once the
	// kernel pipe buffer (~64KiB) fills. os.Pipe yields real *os.File values, so it
	// substitutes for the PTY master directly.
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	s.ptmx = pw
	t.Cleanup(func() { _ = pr.Close() })

	// Wire the real §3.8 response path exactly as spawn does.
	s.startResponseSink(s.ptmx)

	// A burst of CPR queries: each ESC[6n makes x/vt synthesize a ~6-byte reply.
	// 12000 queries => ~72KiB of replies, comfortably past the pipe buffer so the
	// sink's write path saturates and (pre-fix) blocks.
	burst := bytes.Repeat([]byte("\x1b[6n"), 12000)

	pumped := make(chan struct{})
	go func() {
		s.PumpChunkForTest(burst)
		close(pumped)
	}()

	// Give the pump goroutine time to drive the queries through the emulator and
	// (pre-fix) wedge on the blocked sink write while holding s.mu.
	time.Sleep(200 * time.Millisecond)

	killed := make(chan struct{})
	go func() {
		s.Kill()
		close(killed)
	}()

	select {
	case <-killed:
	case <-time.After(5 * time.Second):
		t.Fatal("Kill wedged: response-sink ptmx.Write blocked the session lock (C1 deadlock)")
	}

	// The pump goroutine must also unblock once Kill closes the PTY.
	select {
	case <-pumped:
	case <-time.After(5 * time.Second):
		t.Fatal("pumpStep never returned after Kill (response-sink deadlock)")
	}
}
