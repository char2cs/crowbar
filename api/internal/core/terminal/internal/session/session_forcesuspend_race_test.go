package session

import (
	"bytes"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

const altMarker = "ALTFRAME"

type interleaveModel struct {
	mu  sync.Mutex
	alt bool

	serEntered chan struct{}
	serRelease chan struct{}
}

func (m *interleaveModel) setAlt(v bool) {
	m.mu.Lock()
	m.alt = v
	m.mu.Unlock()
}

func (m *interleaveModel) isAlt() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.alt
}

func (m *interleaveModel) armSerialize() (entered, release chan struct{}) {
	entered = make(chan struct{})
	release = make(chan struct{})
	m.mu.Lock()
	m.serEntered, m.serRelease = entered, release
	m.mu.Unlock()
	return entered, release
}

func (m *interleaveModel) disarmSerialize() {
	m.mu.Lock()
	m.serEntered, m.serRelease = nil, nil
	m.mu.Unlock()
}

func (m *interleaveModel) Write(p []byte) {
	if bytes.Contains(p, []byte(altMarker)) {
		m.setAlt(true)
	}
}

func (m *interleaveModel) Resize(cols, rows int) {}

func (m *interleaveModel) OnForegroundReset() { m.setAlt(false) }

func (m *interleaveModel) Title() string { return "" }
func (m *interleaveModel) Cols() int     { return 80 }
func (m *interleaveModel) Rows() int     { return 24 }
func (m *interleaveModel) HeaderState() (int, int, bool, int) {
	return 80, 24, m.isAlt(), 500
}
func (m *interleaveModel) ModelBytes() int64              { return 0 }
func (m *interleaveModel) Close()                         {}
func (m *interleaveModel) SetResponseSink(func(p []byte)) {}

var _ model.TerminalModel = (*interleaveModel)(nil)

type interleaveSerializer struct{ m *interleaveModel }

func (s interleaveSerializer) Serialize(_ model.TerminalModel) []byte {
	s.m.mu.Lock()
	entered := s.m.serEntered
	release := s.m.serRelease
	s.m.serEntered, s.m.serRelease = nil, nil
	s.m.mu.Unlock()

	if entered != nil {
		close(entered)
		<-release
	}

	if s.m.isAlt() {
		return []byte("\x1b[?1049h" + altMarker)
	}
	return []byte("\x1b[2J\x1b[Hprimary-with-scrollback")
}

var _ model.Serializer = interleaveSerializer{}

func TestRegression_ForceSuspendInterleaveRace(t *testing.T) {
	m := &interleaveModel{}
	swapNewModel(t, func(cols, rows, sb int) (model.TerminalModel, model.Serializer) {
		return m, interleaveSerializer{m: m}
	})

	dir := t.TempDir()
	s, err := newTestSession(t, "sid-forcesuspend-race", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	waitForegroundSampled(t, s)

	notice := []byte("\r\n[crowbar] session suspended\r\n")

	const iterations = 64
	for i := 0; i < iterations; i++ {
		m.setAlt(true)
		entered, release := m.armSerialize()

		blobCh := make(chan []byte, 1)
		go func() { blobCh <- s.ForceSuspendSnapshot(notice) }()

		<-entered

		pumpDone := make(chan struct{})
		go func() {
			s.PumpChunkForTest([]byte(altMarker))
			close(pumpDone)
		}()
		runtime.Gosched()

		close(release)

		blob := <-blobCh
		<-pumpDone

		assert.NotContains(t, string(blob), "\x1b[?1049h",
			"iter %d: the suspended blob must be PRIMARY (no ?1049h), never the racing alt frame", i)
		assert.Contains(t, string(blob), "primary-with-scrollback",
			"iter %d: the suspended blob must serialize the primary screen", i)
		assert.Contains(t, string(blob), "CRWB1 80 24 0 500",
			"iter %d: the suspended blob must carry a primary (alt-bit 0) CRWB1 header", i)
	}

	m.disarmSerialize()
	reSnap, changed := s.Snapshot()
	assert.True(t, changed, "the post-hold racing chunk re-dirtied the model")
	assert.Contains(t, string(reSnap), "\x1b[?1049h",
		"a re-Snapshot after the race captures the racing alt frame — exactly why the engine persists "+
			"the ForceSuspendSnapshot return value verbatim rather than re-Snapshotting")
}
