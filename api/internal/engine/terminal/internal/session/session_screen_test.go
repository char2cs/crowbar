package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

// TestSession_ScreenText_PlaceholderIsUnchanged proves a suspended placeholder (model ==
// nil: no live PTY, no screen to read) reports ("", 0, false) regardless of the caller's
// `since` — never the placeholder's own zero-value screenGen field leaking through, which
// would coincidentally look right only for since==0 and mislead a caller polling with a
// nonzero generation left over from before a suspend.
func TestSession_ScreenText_PlaceholderIsUnchanged(t *testing.T) {
	s := NewPlaceholder("sid-screen-placeholder", "/bin/sh", t.TempDir(), "", nil)

	text, gen, changed := s.ScreenText(0)
	assert.Empty(t, text)
	assert.Zero(t, gen)
	assert.False(t, changed)

	text, gen, changed = s.ScreenText(42)
	assert.Empty(t, text)
	assert.Zero(t, gen)
	assert.False(t, changed, "a stale nonzero `since` must not be mistaken for a match")
}

// TestSession_ScreenText_FirstReadThenUnchanged drives a bare session with a REAL vtModel
// (no PTY, no pump goroutine needed — PumpChunkForTest runs the exact production critical
// section synchronously) through one chunk of output, then proves the (value, changed)
// contract Snapshot already has: the first read at since=0 sees the write (changed=true,
// real text, a nonzero generation), and re-reading with that SAME generation — with nothing
// written in between — reports changed=false and NO text, because the caller already has
// it (this is what makes polling ScreenText cheap for an idle chat).
func TestSession_ScreenText_FirstReadThenUnchanged(t *testing.T) {
	s := newBareSession("sid-screen-first-read", "/bin/sh", t.TempDir(), "")
	m, _ := model.New(80, 24, 0)
	s.model = m

	s.PumpChunkForTest([]byte("hello screen"))

	text, gen1, changed := s.ScreenText(0)
	require.True(t, changed, "the first read after real output must report changed")
	assert.Contains(t, text, "hello screen")
	assert.NotZero(t, gen1)

	text, gen2, changed := s.ScreenText(gen1)
	assert.False(t, changed, "re-reading at the generation already returned must report unchanged")
	assert.Empty(t, text, "an unchanged read must not re-render the screen")
	assert.Equal(t, gen1, gen2)
}

// TestSession_ScreenText_AdvancesOnNewOutput proves a further chunk of PTY-equivalent
// output advances screenGen and makes the next ScreenText read changed again, so a caller
// that cached the generation from its last poll always notices new output rather than
// getting stuck reporting "nothing changed" forever.
func TestSession_ScreenText_AdvancesOnNewOutput(t *testing.T) {
	s := newBareSession("sid-screen-advances", "/bin/sh", t.TempDir(), "")
	m, _ := model.New(80, 24, 0)
	s.model = m

	s.PumpChunkForTest([]byte("first"))
	_, gen1, _ := s.ScreenText(0)

	s.PumpChunkForTest([]byte(" second"))
	text, gen2, changed := s.ScreenText(gen1)
	require.True(t, changed, "a new chunk after the last-seen generation must report changed")
	assert.Greater(t, gen2, gen1, "screenGen must advance on the new write")
	assert.Contains(t, text, "first second")
}

// TestSession_ScreenText_NonScreenReaderModelDegradesGracefully proves a model backend
// that does not implement model.ScreenReader (fakeModel, from session_panic_test.go)
// degrades to "nothing to see" — the same guarded-optional-interface treatment ThemeAware
// and ModelHealth already get elsewhere in this package — rather than panicking on the
// failed type assertion. screenGen still advances (a chunk really was consumed), but no
// text is ever produced for it.
func TestSession_ScreenText_NonScreenReaderModelDegradesGracefully(t *testing.T) {
	s := newBareSession("sid-screen-no-reader", "/bin/sh", t.TempDir(), "")
	s.model = &fakeModel{cols: 80, rows: 24}

	s.PumpChunkForTest([]byte("irrelevant"))

	text, gen, changed := s.ScreenText(0)
	assert.Empty(t, text, "a non-ScreenReader model must never surface text")
	assert.NotZero(t, gen, "screenGen still advances even though no reader can render it")
	assert.False(t, changed)
}
