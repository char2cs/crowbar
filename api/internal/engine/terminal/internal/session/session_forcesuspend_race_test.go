package session

import (
	"bytes"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

// altMarker is the sentinel byte sequence the racing pump chunk feeds to flip the interleave
// model into its alt-screen state. The interleave serializer emits a ?1049h-prefixed frame
// when (and only when) the model is in that alt state.
const altMarker = "ALTFRAME"

// interleaveModel is a hooked model.TerminalModel that tracks a single alt-screen bit. Its
// Write enters alt on the altMarker sentinel; OnForegroundReset (the §11.2 teardown) drives it
// back to primary. All state is guarded by an internal mutex so the live PTY pump goroutine
// (which calls Write under the session lock) never races the test driver, while the SESSION's
// own s.mu remains the thing under test for teardown↔serialize atomicity.
type interleaveModel struct {
	mu  sync.Mutex
	alt bool

	// serEntered/serRelease arm a one-shot barrier INSIDE the paired serializer: the next
	// Serialize closes serEntered (signalling it has been reached while the force-suspend path
	// still holds s.mu) then blocks on serRelease. nil disarms the barrier.
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

// armSerialize installs a fresh one-shot serialize barrier and returns its two channels.
func (m *interleaveModel) armSerialize() (entered, release chan struct{}) {
	entered = make(chan struct{})
	release = make(chan struct{})
	m.mu.Lock()
	m.serEntered, m.serRelease = entered, release
	m.mu.Unlock()
	return entered, release
}

// disarmSerialize removes any armed barrier so a later Serialize runs without blocking.
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

// OnForegroundReset is the non-blocking §11.2 teardown: it drives the model to the primary
// buffer. It is deliberately NOT the barrier (the live pump fires OnForegroundReset on the
// startup foreground edge; blocking here would deadlock the pump). The barrier lives in the
// serializer instead, which the live pump never calls.
func (m *interleaveModel) OnForegroundReset() { m.setAlt(false) }

func (m *interleaveModel) PendingInput() []byte { return nil }
func (m *interleaveModel) Title() string        { return "" }
func (m *interleaveModel) Cols() int            { return 80 }
func (m *interleaveModel) Rows() int            { return 24 }
func (m *interleaveModel) HeaderState() (int, int, bool, int) {
	return 80, 24, m.isAlt(), 500
}
func (m *interleaveModel) ModelBytes() int64 { return 0 }
func (m *interleaveModel) Close()            {}

var _ model.TerminalModel = (*interleaveModel)(nil)

// interleaveSerializer emits a primary or alt redraw depending on the paired model's alt bit,
// and fires the model's one-shot serialize barrier so the test can deterministically hold the
// force-suspend path mid-serialize (still under s.mu) while it launches a racing pump chunk.
type interleaveSerializer struct{ m *interleaveModel }

func (s interleaveSerializer) Serialize(_ model.TerminalModel) []byte {
	s.m.mu.Lock()
	entered := s.m.serEntered
	release := s.m.serRelease
	s.m.serEntered, s.m.serRelease = nil, nil // consume: fire at most once per arm
	s.m.mu.Unlock()

	if entered != nil {
		close(entered) // serialize reached — the force-suspend path holds s.mu here
		<-release      // block until the test has launched the racing chunk and released us
	}

	if s.m.isAlt() {
		return []byte("\x1b[?1049h" + altMarker)
	}
	return []byte("\x1b[2J\x1b[Hprimary-with-scrollback")
}

var _ model.Serializer = interleaveSerializer{}

// TestRegression_ForceSuspendInterleaveRace guards the §11.2 single-hold atomicity. Run under
// -race. ForceSuspendSnapshot fuses teardown (OnForegroundReset → primary), notice injection,
// and serialize into ONE s.mu critical section, so a live pumpStep chunk that re-paints the
// alt screen can NOT interleave between the teardown and the serialize and leak alt content
// into the suspended blob.
//
// Determinism: a one-shot barrier inside the serializer parks the force-suspend path
// mid-serialize while it still holds s.mu; the test then launches a racing PumpChunkForTest
// (an alt repaint that does NOT re-emit ?1049h, exactly as htop/vim would) which must block on
// s.mu, and only then releases the barrier. The persisted blob is therefore PRIMARY on every
// iteration regardless of the racing chunk. Over many iterations the race detector exercises
// the contended hand-off.
//
// It also pins the "persist the returned blob, never re-Snapshot" half of §11.2: after the
// hold releases, the racing chunk DOES land and re-alts the model, so a fresh Snapshot()
// diverges to the alt frame. The engine's force-suspend path persists the blob ForceSuspendSnapshot
// returns verbatim (terminal.go suspend()); a re-Snapshot would instead persist that racing
// alt frame and corrupt the restore — which this test proves would happen.
func TestRegression_ForceSuspendInterleaveRace(t *testing.T) {
	m := &interleaveModel{}
	swapNewModel(t, func(cols, rows, sb int) (model.TerminalModel, model.Serializer) {
		return m, interleaveSerializer{m: m}
	})

	dir := t.TempDir()
	s, err := newTestSession(t, "sid-forcesuspend-race", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	// Let the shell settle so the live pump has already sampled and latched the foreground
	// process group (firing OnForegroundReset its one startup time); after this the pump never
	// fires OnForegroundReset again and, sitting idle on ptmx.Read, issues no further model
	// writes — so the only model mutations during the loop are the ones the test drives.
	waitIdleOrSkip(t, s)

	notice := []byte("\r\n[crowbar] session suspended\r\n")

	const iterations = 64
	for i := 0; i < iterations; i++ {
		m.setAlt(true) // an alt-screen app is live at the start of the iteration
		entered, release := m.armSerialize()

		blobCh := make(chan []byte, 1)
		go func() { blobCh <- s.ForceSuspendSnapshot(notice) }()

		<-entered // force-suspend is mid-serialize, holding s.mu

		pumpDone := make(chan struct{})
		go func() {
			s.PumpChunkForTest([]byte(altMarker)) // races for s.mu; must wait for the hold
			close(pumpDone)
		}()
		runtime.Gosched() // best-effort: let the racing pump reach s.mu.Lock (not load-bearing)

		close(release) // serialize finishes on the PRIMARY screen, under the single hold

		blob := <-blobCh
		<-pumpDone

		assert.NotContains(t, string(blob), "\x1b[?1049h",
			"iter %d: the suspended blob must be PRIMARY (no ?1049h), never the racing alt frame", i)
		assert.Contains(t, string(blob), "primary-with-scrollback",
			"iter %d: the suspended blob must serialize the primary screen", i)
		assert.Contains(t, string(blob), "CRWB1 80 24 0 500",
			"iter %d: the suspended blob must carry a primary (alt-bit 0) CRWB1 header", i)
	}

	// The final iteration's racing chunk landed AFTER the hold released, re-alting the model.
	// A re-Snapshot now captures the alt frame — proving the engine MUST persist the blob
	// ForceSuspendSnapshot returned (it does) and must NOT re-Snapshot after the hold.
	m.disarmSerialize()
	reSnap, changed := s.Snapshot()
	assert.True(t, changed, "the post-hold racing chunk re-dirtied the model")
	assert.Contains(t, string(reSnap), "\x1b[?1049h",
		"a re-Snapshot after the race captures the racing alt frame — exactly why the engine persists "+
			"the ForceSuspendSnapshot return value verbatim rather than re-Snapshotting")
}
