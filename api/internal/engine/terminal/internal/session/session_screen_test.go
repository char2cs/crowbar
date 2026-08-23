package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

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

func TestSession_ScreenText_NonScreenReaderModelDegradesGracefully(t *testing.T) {
	s := newBareSession("sid-screen-no-reader", "/bin/sh", t.TempDir(), "")
	s.model = &fakeModel{cols: 80, rows: 24}

	s.PumpChunkForTest([]byte("irrelevant"))

	text, gen, changed := s.ScreenText(0)
	assert.Empty(t, text, "a non-ScreenReader model must never surface text")
	assert.NotZero(t, gen, "screenGen still advances even though no reader can render it")
	assert.False(t, changed)
}
