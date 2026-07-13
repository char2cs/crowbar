package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

// orderModel is the "hooked model" half of the TestRegression_LivePathUnchangedRaw fixture
// (spec §8.2/§11.1 site #1/§13.3). It performs no emulation; it records, in order, every
// mutation the session drives through it during pumpStep — each Write (capturing the bytes it
// saw and how many frames were already enqueued on the watched client channel at that exact
// instant) and each OnForegroundReset. The fan-out peek is what proves the model write trails
// the live fan-out: pumpStep runs entirely under s.mu in the calling goroutine, so by the time
// Write runs, fanOutLocked has already pushed the raw chunk into watch iff len(watch) ≥ 1.
type orderModel struct {
	events    []string // ordered mutation log: "write" / "foreground-reset"
	writes    [][]byte // bytes seen by each Write, in order
	fanoutLen []int    // len(watch) at the instant of each Write
	watch     <-chan OutputFrame
}

func (m *orderModel) Write(p []byte) {
	m.events = append(m.events, "write")
	b := make([]byte, len(p))
	copy(b, p)
	m.writes = append(m.writes, b)
	m.fanoutLen = append(m.fanoutLen, len(m.watch))
}

func (m *orderModel) OnForegroundReset()                 { m.events = append(m.events, "foreground-reset") }
func (m *orderModel) Resize(int, int)                    {}
func (m *orderModel) PendingInput() []byte               { return nil }
func (m *orderModel) Title() string                      { return "" }
func (m *orderModel) Cols() int                          { return 80 }
func (m *orderModel) Rows() int                          { return 24 }
func (m *orderModel) HeaderState() (int, int, bool, int) { return 80, 24, false, 0 }
func (m *orderModel) ModelBytes() int64                  { return 0 }
func (m *orderModel) Close()                             {}
func (m *orderModel) SetResponseSink(func(p []byte))     {}

var _ model.TerminalModel = (*orderModel)(nil)

// TestRegression_LivePathUnchangedRaw pins the load-bearing pumpStep ordering the spec makes a
// HARD REQUIREMENT (§8.2 step (3) / §11.1 site #1 / §13.3): inside one s.mu critical section,
// (1) the RAW chunk is fanned out to live clients byte-for-byte FIRST, (2) the model write
// runs strictly AFTER the fan-out enqueue, and (3) the debounced foreground sample — the
// TIOCGPGRP probe and any app-death-edge teardown model mutation — runs strictly LAST, after
// BOTH fanOutLocked and writeModelLocked, so neither the ioctl nor the teardown can ever
// precede or delay the live fan-out (NG1: zero added per-chunk latency).
//
// It is driven with a hooked model (orderModel) and a hooked foreground sampler (the
// sampleForegroundLocked seam) on a PTY-less, pump-less bare session, so the ONLY thing that
// touches pumpStep is the single explicit PumpChunkForTest call below — making the ordering
// log exact and race-free. A future refactor that moved writeModelLocked before fanOutLocked,
// or sampled the foreground before fan-out, would fail here with an otherwise-green suite.
func TestRegression_LivePathUnchangedRaw(t *testing.T) {
	// Bare session: no PTY, no pump goroutine. We inject the hooked model directly and replace
	// the foreground-sampler seam with a hooked sampler that records that it ran and drives the
	// model's teardown reset exactly as the §11.1 app-death edge's checkForegroundResetLocked
	// would. Both the record and the teardown mutation must land STRICTLY LAST.
	s := newBareSession("sid-livepath", "/bin/sh", t.TempDir(), "")
	om := &orderModel{}
	s.model = om

	var fgSampled bool
	s.sampleForegroundLocked = func() {
		fgSampled = true
		s.mutateModelLocked(s.model.OnForegroundReset)
	}

	// Attach a live client so fanOutLocked has a real destination. The bare session's
	// serializer is nil, so Attach's serializeLocked panics and is recovered to a nil redraw —
	// the client channel starts EMPTY (no prefill), which the fan-out ordering peek relies on.
	ch, err := s.Attach()
	require.NoError(t, err)
	om.watch = ch

	// Force the 250 ms-debounced foreground sample to fire on this single pump.
	s.lastFgSampleAt = time.Time{}

	// A chunk that any transformation would alter byte-for-byte: an SGR run, invalid UTF-8
	// bytes, and a dangling/incomplete escape at the very tail.
	chunk := []byte("hi \x1b[31mred\x1b[0m\xff\xfe tail\x1b[")
	s.PumpChunkForTest(chunk)

	// (1) RAW fan-out, byte-for-byte: the single live client received the chunk verbatim with
	// no transformation.
	frame, ok := waitFrame(t, ch)
	require.True(t, ok, "live client must receive the pumped chunk")
	assert.Equal(t, chunk, frame.Data, "fan-out must deliver the RAW chunk byte-for-byte")

	// (2) The model write ran AFTER the fan-out enqueue: at the instant the model's first Write
	// executed, the raw chunk was already sitting in the client's channel (len ≥ 1).
	require.NotEmpty(t, om.writes, "the model must have been written")
	assert.Equal(t, chunk, om.writes[0], "the model is fed the same raw chunk")
	require.NotEmpty(t, om.fanoutLen)
	assert.GreaterOrEqual(t, om.fanoutLen[0], 1,
		"fanOutLocked must enqueue the raw chunk BEFORE writeModelLocked (NG1: zero added latency)")

	// (3) The foreground sample and its teardown model mutation ran STRICTLY LAST — after both
	// fanOutLocked and writeModelLocked. The ordered model log for this chunk is exactly the
	// chunk write followed by the teardown reset; the sampler fired.
	assert.True(t, fgSampled, "pumpStep must run the debounced foreground sample")
	assert.Equal(t, []string{"write", "foreground-reset"}, om.events,
		"ordering must be model.Write(chunk) THEN the foreground sample's teardown reset — sampler strictly last")

	// (4) NOTHING ELSE was fanned out for that chunk: neither the model write nor the teardown
	// may reach the wire. That negative is proved by a SEQUENCE BARRIER, not by watching an idle
	// channel for an arbitrary window. Pump a SECOND, distinct chunk: the raw path must fan it
	// out verbatim in its turn. The client's channel is FIFO, so had the first chunk's model
	// write or teardown produced a frame, that frame would necessarily sit AHEAD of chunk two
	// and be the next thing received. Receiving chunk two verbatim therefore proves nothing was
	// emitted in between — with no clock, and no window to guess at.
	second := []byte("second\x1b[32m")
	s.PumpChunkForTest(second)
	next, ok := waitFrame(t, ch)
	require.True(t, ok, "live client must receive the second pumped chunk")
	assert.Equal(t, second, next.Data,
		"the next frame must be the second RAW chunk: nothing else (model write, teardown) may fan out")
}
