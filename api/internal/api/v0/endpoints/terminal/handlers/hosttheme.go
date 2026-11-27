package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
)

type hostThemeReq struct {
	Bg string `json:"bg"`
	Fg string `json:"fg"`
}

func (h *Handlers) HostTheme(ctx *gin.Context) {
	bg, fg := h.termEng.HostTheme()
	libs.WriteQueryOK(ctx, hostThemeReq{Bg: engineterminal.FormatHexColor(bg), Fg: engineterminal.FormatHexColor(fg)})
}

func (h *Handlers) SetHostTheme(ctx *gin.Context) {
	var req hostThemeReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	bg, fg := engineterminal.ParseHexColor(req.Bg), engineterminal.ParseHexColor(req.Fg)
	if bg == nil && fg == nil {
		libs.WriteErr(ctx, http.StatusBadRequest,
			`theme requires at least one parseable colour ("#rgb", "#rrggbb" or "#rrggbbaa")`)
		return
	}
	h.termEng.SetHostTheme(bg, fg)

	ctx.Status(http.StatusNoContent)
}
