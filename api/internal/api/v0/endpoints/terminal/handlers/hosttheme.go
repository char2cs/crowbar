package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
)

// hostThemeReq is the host terminal's default colours in the per-session frame's "#rrggbb" form.
// It has no dark flag: polarity only picks the DEC 2031 report for an already-running app, which
// is the per-session channel's job.
type hostThemeReq struct {
	Bg string `json:"bg"`
	Fg string `json:"fg"`
}

// HostTheme handles GET /v0/settings/terminal/theme: the pair every later PTY answers with, ""
// for a channel never pushed. The frontend pushes only a pair that differs from it.
func (h *Handlers) HostTheme(ctx *gin.Context) {
	bg, fg := h.termEng.HostTheme()
	libs.WriteQueryOK(ctx, hostThemeReq{Bg: engineterminal.FormatHexColor(bg), Fg: engineterminal.FormatHexColor(fg)})
}

// SetHostTheme handles PUT /v0/settings/terminal/theme, recording the colours every later PTY
// answers OSC 10/11 with: a CLI often queries before any client can attach and push a theme.
// Idempotent; the frontend pushes at boot and on each theme change, and the newest wins.
func (h *Handlers) SetHostTheme(ctx *gin.Context) {
	var req hostThemeReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	bg, fg := engineterminal.ParseHexColor(req.Bg), engineterminal.ParseHexColor(req.Fg)
	if bg == nil && fg == nil {
		// Neither channel parsed. Refusing is the honest answer: accepting would report
		// success for a call that changed nothing, and the caller would have no way to tell
		// a typo'd colour from a stored one.
		libs.WriteErr(ctx, http.StatusBadRequest,
			`theme requires at least one parseable colour ("#rgb", "#rrggbb" or "#rrggbbaa")`)
		return
	}
	h.termEng.SetHostTheme(bg, fg)

	ctx.Status(http.StatusNoContent)
}
