package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
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
func (m *orderModel) Title() string                      { return "" }
func (m *orderModel) Cols() int                          { return 80 }
func (m *orderModel) Rows() int                          { return 24 }
func (m *orderModel) HeaderState() (int, int, bool, int) { return 80, 24, false, 0 }
func (m *orderModel) ModelBytes() int64                  { return 0 }
func (m *orderModel) Close()                             {}
func (m *orderModel) SetResponseSink(func(p []byte))     {}

var _ model.TerminalModel = (*orderModel)(nil)

// TestRegression_PumpStepSamplesForegroundLast pins the pumpStep ordering §11.1 site #1
// requires: inside one s.mu critical section the chunk is written to the model FIRST and the
// debounced foreground sample — the TIOCGPGRP probe and any app-death-edge teardown model
// mutation — runs strictly LAST, so neither can precede or delay the chunk's own frame.
func TestRegression_PumpStepSamplesForegroundLast(t *testing.T) {
	s := newBareSession("sid-livepath", "/bin/sh", t.TempDir(), "")
	om := &orderModel{}
	s.model = om

	var fgSampled bool
	s.sampleForegroundLocked = func() {
		fgSampled = true
		s.mutateModelLocked(s.model.OnForegroundReset)
	}
	s.lastFgSampleAt = time.Time{} // force the debounced sample on this pump

	chunk := []byte("hi \x1b[31mred\x1b[0m\xff\xfe tail\x1b[")
	// Park the frame clock so this chunk's emit is deferred to a trailing timer that cannot
	// fire during the test: only the model write and the sample run synchronously.
	s.emitter = model.NewDiffEmitter()
	s.lastEmitAt = time.Now().Add(time.Hour)
	t.Cleanup(func() {
		s.mu.Lock()
		s.stopEmitTimerLocked()
		s.mu.Unlock()
	})
	s.PumpChunkForTest(chunk)

	require.NotEmpty(t, om.writes, "the model must have been written")
	assert.Equal(t, chunk, om.writes[0], "the model is fed the chunk verbatim")
	assert.True(t, fgSampled, "pumpStep must run the debounced foreground sample")
	assert.Equal(t, []string{"write", "foreground-reset"}, om.events,
		"ordering must be model.Write(chunk) THEN the foreground sample's teardown reset")
}
