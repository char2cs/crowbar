package handlers_test

import (
	"image/color"
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

// recordingEngine is stubEngine plus a record of what SetHostTheme was handed, so the
// endpoint's job — parse the wire colours and pass them on — can be asserted end to end.
type recordingEngine struct {
	stubEngine

	mu     sync.Mutex
	calls  int
	bg, fg color.Color
}

func (r *recordingEngine) SetHostTheme(
	bg color.Color,
	fg color.Color,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.bg, r.fg = bg, fg
}

func (r *recordingEngine) snapshot() (calls int, bg, fg color.Color) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.bg, r.fg
}

func newHostThemeRouter(eng handlers.TerminalEngine) *gin.Engine {
	r := gin.New()
	h := handlers.New(eng, stubProfiles{}, stubReader{}, &spyBroadcaster{})
	r.Group("/v0").PUT("/settings/terminal/theme", h.SetHostTheme)
	return r
}

func rgb(c color.Color) (r, g, b uint8) {
	rr, gg, bb, _ := c.RGBA()
	return uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8)
}

// TestSetHostTheme_ParsesAndForwards is the endpoint half of the Codex theming fix: the
// frontend's resolved "#rrggbb" pair must reach the engine as colours, so the next PTY the
// daemon spawns is born answering an OSC 11 query with them.
func TestSetHostTheme_ParsesAndForwards(t *testing.T) {
	eng := &recordingEngine{}
	rec := doTerminal(newHostThemeRouter(eng), http.MethodPut, "/v0/settings/terminal/theme",
		map[string]string{"bg": "#faf9f5", "fg": "#141414"})

	require.Equal(t, http.StatusNoContent, rec.Code)

	calls, bg, fg := eng.snapshot()
	require.Equal(t, 1, calls)
	require.NotNil(t, bg)
	require.NotNil(t, fg)

	r, g, b := rgb(bg)
	assert.Equal(t, [3]uint8{0xfa, 0xf9, 0xf5}, [3]uint8{r, g, b})
	r, g, b = rgb(fg)
	assert.Equal(t, [3]uint8{0x14, 0x14, 0x14}, [3]uint8{r, g, b})
}

// TestSetHostTheme_LastPushWins mirrors what a theme switch does: the endpoint is
// unconditional and idempotent, so the newest push is simply what later sessions inherit.
func TestSetHostTheme_LastPushWins(t *testing.T) {
	eng := &recordingEngine{}
	r := newHostThemeRouter(eng)

	require.Equal(t, http.StatusNoContent,
		doTerminal(r, http.MethodPut, "/v0/settings/terminal/theme",
			map[string]string{"bg": "#faf9f5", "fg": "#141414"}).Code)
	require.Equal(t, http.StatusNoContent,
		doTerminal(r, http.MethodPut, "/v0/settings/terminal/theme",
			map[string]string{"bg": "#1e1e1e", "fg": "#ffffff"}).Code)

	calls, bg, _ := eng.snapshot()
	require.Equal(t, 2, calls)
	rr, gg, bb := rgb(bg)
	assert.Equal(t, [3]uint8{0x1e, 0x1e, 0x1e}, [3]uint8{rr, gg, bb})
}

// TestSetHostTheme_RejectsUnusableColours: a payload where neither channel parses changed
// nothing, so reporting success would tell the caller a typo'd colour had been stored.
func TestSetHostTheme_RejectsUnusableColours(t *testing.T) {
	for _, body := range []map[string]string{
		{"bg": "", "fg": ""},
		{"bg": "not-a-colour", "fg": "#12345"},
	} {
		eng := &recordingEngine{}
		rec := doTerminal(newHostThemeRouter(eng), http.MethodPut,
			"/v0/settings/terminal/theme", body)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "body %v", body)
		calls, _, _ := eng.snapshot()
		assert.Zero(t, calls, "a rejected payload must not reach the engine")
	}
}

// TestSetHostTheme_AcceptsOneParseableChannel: a nil channel is a documented no-op downstream
// (model.SetDefaultColors leaves it unchanged), so one good colour is a usable push.
func TestSetHostTheme_AcceptsOneParseableChannel(t *testing.T) {
	eng := &recordingEngine{}
	rec := doTerminal(newHostThemeRouter(eng), http.MethodPut, "/v0/settings/terminal/theme",
		map[string]string{"bg": "#faf9f5", "fg": "garbage"})

	require.Equal(t, http.StatusNoContent, rec.Code)
	calls, bg, fg := eng.snapshot()
	require.Equal(t, 1, calls)
	require.NotNil(t, bg)
	assert.Nil(t, fg, "an unparseable channel must arrive as nil, not as an invented colour")
}
