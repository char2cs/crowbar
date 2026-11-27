package session

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

type panicCtl struct {
	resize    atomic.Bool
	serialize atomic.Bool
	write     atomic.Bool
}

type fakeModel struct {
	ctl        *panicCtl
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

type panicSerializer struct{ ctl *panicCtl }

func (s panicSerializer) Serialize(m model.TerminalModel) []byte {
	if s.ctl != nil && s.ctl.serialize.Load() {
		panic("injected serialize panic")
	}
	return []byte("REDRAW")
}

func swapNewModel(
	t *testing.T,
	fn func(cols, rows, sb int) (model.TerminalModel, model.Serializer),
) {
	t.Helper()
	orig := newModel
	newModel = fn
	t.Cleanup(func() { newModel = orig })
}

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

	ctl.resize.Store(true)
	require.NoError(t, s.Resize(100, 40), "Resize must swallow the model panic and return nil")
	_, pp1 := s.Health()
	assert.Greater(t, pp1, pp0, "a Resize-drain panic must bump modelPanics (visible via Health)")
	requireOpen("after resize panic")
	ctl.resize.Store(false)

	ctl.serialize.Store(true)
	ch, err := s.Attach()
	require.NoError(t, err, "Attach must succeed even when serialize panics")
	_, pp2 := s.Health()
	assert.Greater(t, pp2, pp1, "an Attach-serialize panic must bump modelPanics")
	requireOpen("after attach-serialize panic")
	s.Detach(ch)

	ctl.serialize.Store(false)
	ch2, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch2)
	f, ok := waitFrame(t, ch2)
	require.True(t, ok, "post-recovery attach must deliver a redraw frame")
	assert.Equal(t, "REDRAW", string(f.Data))
	require.NoError(t, s.Write([]byte("echo ok\n")), "post-recovery Write must succeed")
	requireOpen("after recovery")
}

func TestSession_HealthSurfacesModelHealth(t *testing.T) {
	s := newBareSession("sid-health", "/bin/sh", t.TempDir(), "")
	s.model = &fakeModel{cols: 80, rows: 24, degraded: true, panics: 3}

	degraded, panics := s.Health()
	assert.True(t, degraded, "a degraded model must surface through Session.Health()")
	assert.Equal(t, 3, panics, "ModelHealth.ParsePanics() must aggregate into Health()")
}

func TestSession_HealthCleanModel(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-health-clean", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	degraded, panics := s.Health()
	assert.False(t, degraded, "a healthy session must not be degraded")
	assert.Zero(t, panics, "a healthy session must report zero parse panics")
}

func TestBackstopPanic_RebuildsModelAndKeyframes(t *testing.T) {
	s := newBareSession("sid-backstop", "/bin/sh", t.TempDir(), "")
	m, ser := model.New(80, 24, 200)
	s.model, s.serializer = m, ser
	s.cols, s.rows, s.scrollback = 80, 24, 200

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, ok := waitFrame(t, ch)
	require.True(t, ok)
	s.PumpChunkForTest([]byte("BEFORE"))
	_, ok = waitFrame(t, ch)
	require.True(t, ok)

	s.mu.Lock()
	s.mutateModelLocked(func() { panic("injected model panic") })
	rebuilt := s.model != m
	s.mu.Unlock()
	require.True(t, rebuilt, "a caught model panic must rebuild the model")
	_, panics := s.Health()
	assert.Equal(t, 1, panics)

	s.PumpChunkForTest([]byte("AFTER"))
	f, ok := waitFrame(t, ch)
	require.True(t, ok)
	assert.True(t, f.Snapshot, "the first frame after the rebuild must be a keyframe")
	assert.Contains(t, string(f.Data), "AFTER")
	assert.NotContains(t, string(f.Data), "BEFORE", "the rebuilt model starts from a blank screen")
}
