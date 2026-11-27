package api

import (
	"net/http"
	"net/http/pprof"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func mountDebug(
	router *gin.Engine,
) {
	handler := func(c *gin.Context) {
		if c.Request.TLS != nil {
			libs.WriteErr(c, http.StatusForbidden, "pprof is only served over the local socket")
			return
		}
		switch c.Param("rest") {
		case "/cmdline":
			pprof.Cmdline(c.Writer, c.Request)
		case "/profile":
			pprof.Profile(c.Writer, c.Request)
		case "/symbol":
			pprof.Symbol(c.Writer, c.Request)
		case "/trace":
			pprof.Trace(c.Writer, c.Request)
		default:
			pprof.Index(c.Writer, c.Request)
		}
	}
	router.GET("/debug/pprof/*rest", handler)
	router.POST("/debug/pprof/*rest", handler)
}
