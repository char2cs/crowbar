package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// hostThemeReq is the host terminal's resolved default colours, in the same "#rrggbb" wire
// form the per-session theme frame already carries (resolve-css-color.ts output).
//
// There is no `dark` field, and that omission is deliberate rather than an oversight: the
// polarity flag exists only to choose which DEC 2031 CSI ?997;n report to inject into an
// ALREADY-RUNNING subscribed app, which is the per-session channel's job. Nothing born from
// this endpoint has a foreground app yet, so a polarity here would be a field with no reader.
type hostThemeReq struct {
	Bg string `json:"bg"`
	Fg string `json:"fg"`
}

// SetHostTheme handles PUT /v0/settings/terminal/theme: it records the host terminal's
// light/dark colours as what every SUBSEQUENTLY SPAWNED PTY answers an OSC 10/11 query with.
//
// This is the half of theme propagation the per-session channel structurally cannot cover.
// A vendor CLI is exec'd by the daemon at session-creation time, so it is already running —
// and, if it detects light/dark by querying the background, has usually already asked —
// before any client can attach and push a theme down the session's socket. Claude Code
// survived that ordering only because it also subscribes to DEC 2031 and re-queries when the
// late push notifies it; codex 0.146.0 has no 2031 support, so the answer it got at birth was
// final. This endpoint is what makes that first answer correct.
//
// It is idempotent and unconditional — the frontend pushes at boot and on every theme change,
// and the newest push simply wins.
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
