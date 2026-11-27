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

func (r *recordingEngine) HostTheme() (bg, fg color.Color) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bg, r.fg
}

func (r *recordingEngine) snapshot() (calls int, bg, fg color.Color) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.bg, r.fg
}

func newHostThemeRouter(eng handlers.TerminalEngine) *gin.Engine {
	r := gin.New()
	h := handlers.New(eng, stubProfiles{})
	r.Group("/v0").PUT("/settings/terminal/theme", h.SetHostTheme)
	r.Group("/v0").GET("/settings/terminal/theme", h.HostTheme)
	return r
}

func rgb(c color.Color) (r, g, b uint8) {
	rr, gg, bb, _ := c.RGBA()
	return uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8)
}

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

func TestHostTheme_ReadsBackTheHeldColours(t *testing.T) {
	r := newHostThemeRouter(&recordingEngine{})
	rec := doTerminal(r, http.MethodGet, "/v0/settings/terminal/theme", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"success":true,"data":{"bg":"","fg":""}}`, rec.Body.String())

	doTerminal(r, http.MethodPut, "/v0/settings/terminal/theme",
		map[string]string{"bg": "#FAF9F5", "fg": "#141414"})
	rec = doTerminal(r, http.MethodGet, "/v0/settings/terminal/theme", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"success":true,"data":{"bg":"#faf9f5","fg":"#141414"}}`, rec.Body.String())
}
