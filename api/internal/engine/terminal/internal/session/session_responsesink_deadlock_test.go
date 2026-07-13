//go:build !windows

package session

import (
	"bytes"
	"os"
	"testing"

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

	// Wait for the pump to finish driving the burst before killing. This is THE assertion, and
	// it needs no failsafe: on the FIXED code the bounded-queue sink never blocks the model
	// write, so pumpStep returns and `pumped` closes. On the REGRESSED (direct-write) sink
	// pumpStep wedges inside its ptmx.Write on the full pipe while HOLDING s.mu — the wedge the
	// whole test exists to catch — and `pumped` never closes. That is a hang, and `go test
	// -timeout` reports it with the goroutine dump pointing straight at the blocked
	// pumpStep/ptmx.Write frame: a strictly better diagnostic than a bespoke "2s elapsed"
	// fallthrough, which would have gone on to blame the SUBSEQUENT Kill for a deadlock the
	// pump had already caused.
	<-pumped

	killed := make(chan struct{})
	go func() {
		s.Kill()
		close(killed)
	}()

	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-killed

	// The pump goroutine must also unblock once Kill closes the PTY.
	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-pumped
}
