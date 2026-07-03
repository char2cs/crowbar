package session

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

// panicCtl toggles which model-access path panics, so one fake model can drive both the §8.5
// Resize (mutateModelLocked) and Attach-serialize (serializeLocked) session recover backstops
// from a single test. The real vtModel recovers Write panics internally, so those session
// backstops are otherwise unreachable through Session.Resize/Attach.
type panicCtl struct {
	resize    atomic.Bool
	serialize atomic.Bool
	write     atomic.Bool
}

// fakeModel is a no-op model.TerminalModel + model.ModelHealth used to exercise the session's
// parse-health surface and panic isolation. Its Resize panics on demand (the §8.5 Resize
// path); Write/OnForegroundReset are inert so the live pump never bumps modelPanics on its
// own and the per-panic count stays exact. degraded/panics seed the ModelHealth read.
type fakeModel struct {
	ctl        *panicCtl // nil ⇒ Resize never panics
	cols, rows int
	degraded   bool
	panics     int
}

func (m *fakeModel) Write(p []byte) {
	if m.ctl != nil && m.ctl.write.Load() {
		panic("injected write panic")
	}
}

func (m *fakeModel) Resize(cols, rows int) {
	if m.ctl != nil && m.ctl.resize.Load() {
		panic("injected resize panic")
	}
	m.cols, m.rows = cols, rows
}

func (m *fakeModel) OnForegroundReset()                 {}
func (m *fakeModel) PendingInput() []byte               { return nil }
func (m *fakeModel) Title() string                      { return "" }
func (m *fakeModel) Cols() int                          { return m.cols }
func (m *fakeModel) Rows() int                          { return m.rows }
func (m *fakeModel) HeaderState() (int, int, bool, int) { return m.cols, m.rows, false, 0 }
func (m *fakeModel) ModelBytes() int64                  { return 0 }
func (m *fakeModel) Close()                             {}
func (m *fakeModel) Degraded() bool                     { return m.degraded }
func (m *fakeModel) ParsePanics() int                   { return m.panics }
func (m *fakeModel) SetResponseSink(func(p []byte))     {}

var (
	_ model.TerminalModel = (*fakeModel)(nil)
	_ model.ModelHealth   = (*fakeModel)(nil)
)

// panicSerializer panics from Serialize on demand to drive the Attach serializeLocked
// backstop; otherwise it emits a fixed sentinel redraw.
type panicSerializer struct{ ctl *panicCtl }

func (s panicSerializer) Serialize(m model.TerminalModel) []byte {
	if s.ctl != nil && s.ctl.serialize.Load() {
		panic("injected serialize panic")
	}
	return []byte("REDRAW")
}

// swapNewModel substitutes the session's model-construction seam for the duration of a test.
func swapNewModel(
	t *testing.T,
	fn func(cols, rows, sb int) (model.TerminalModel, model.Serializer),
) {
	t.Helper()
	orig := newModel
	newModel = fn
	t.Cleanup(func() { newModel = orig })
}

// TestRegression_ModelPanicOnResizeAndAttachStillServes pins §8.5: a panic on the Resize
// drain (mutateModelLocked) and on the Attach serialize (serializeLocked) is recovered, bumps
// the session's modelPanics counter (now observable via Health/Stats — §9.4), never closes
// done, and leaves the session serving the next Attach/Write. Deleting either recover backstop
// would surface the panic to pump()/the caller and fail this test.
func TestRegression_ModelPanicOnResizeAndAttachStillServes(t *testing.T) {
	ctl := &panicCtl{}
	swapNewModel(t, func(cols, rows, sb int) (model.TerminalModel, model.Serializer) {
		return &fakeModel{ctl: ctl, cols: cols, rows: rows}, panicSerializer{ctl: ctl}
	})

	dir := t.TempDir()
	s, err := newTestSession(t, "sid-modelpanic", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	requireOpen := func(msg string) {
		t.Helper()
		select {
		case <-s.Done():
			t.Fatalf("session done closed after recovered panic: %s", msg)
		default:
		}
	}

	_, pp0 := s.Health()

	// (1) Resize-drain panic → mutateModelLocked recovers; Resize still returns nil (the
	// pty.Setsize succeeded), modelPanics bumps, the session stays live.
	ctl.resize.Store(true)
	require.NoError(t, s.Resize(100, 40), "Resize must swallow the model panic and return nil")
	_, pp1 := s.Health()
	assert.Greater(t, pp1, pp0, "a Resize-drain panic must bump modelPanics (visible via Health)")
	requireOpen("after resize panic")
	ctl.resize.Store(false)

	// (2) Attach-serialize panic → serializeLocked recovers and yields a nil redraw; Attach
	// still registers the client and returns its channel, modelPanics bumps again.
	ctl.serialize.Store(true)
	ch, err := s.Attach()
	require.NoError(t, err, "Attach must succeed even when serialize panics")
	_, pp2 := s.Health()
	assert.Greater(t, pp2, pp1, "an Attach-serialize panic must bump modelPanics")
	requireOpen("after attach-serialize panic")
	// The recovered serialize yields a nil redraw; the client is still registered (live
	// fan-out continues), so we only assert the session survived, not the exact frames.
	s.Detach(ch)

	// (3) The session STILL SERVES: a clean attach now yields the sentinel redraw and Write
	// succeeds.
	ctl.serialize.Store(false)
	ch2, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch2)
	f, ok := waitFrame(t, ch2, time.Second)
	require.True(t, ok, "post-recovery attach must deliver a redraw frame")
	assert.Equal(t, "REDRAW", string(f.Data))
	require.NoError(t, s.Write([]byte("echo ok\n")), "post-recovery Write must succeed")
	requireOpen("after recovery")
}

// TestSession_HealthSurfacesModelHealth proves Session.Health() threads the optional
// model.ModelHealth surface through to the engine's Stats observability (§9.4): a degraded
// model with recovered parse panics is reported as degraded with the panic count, so a
// blanked-and-reparsed session is no longer invisible to operators.
func TestSession_HealthSurfacesModelHealth(t *testing.T) {
	swapNewModel(t, func(cols, rows, sb int) (model.TerminalModel, model.Serializer) {
		return &fakeModel{cols: cols, rows: rows, degraded: true, panics: 3}, panicSerializer{}
	})

	dir := t.TempDir()
	s, err := newTestSession(t, "sid-health", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	degraded, panics := s.Health()
	assert.True(t, degraded, "a degraded model must surface through Session.Health()")
	assert.Equal(t, 3, panics, "ModelHealth.ParsePanics() must aggregate into Health()")
}

// TestSession_HealthCleanModel proves a healthy session reports clean health: not degraded
// and zero parse panics.
func TestSession_HealthCleanModel(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-health-clean", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	degraded, panics := s.Health()
	assert.False(t, degraded, "a healthy session must not be degraded")
	assert.Zero(t, panics, "a healthy session must report zero parse panics")
}
