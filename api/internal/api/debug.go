// debug.go mounts the Go runtime's pprof surface on the daemon router.
package api

import (
	"net/http"
	"net/http/pprof"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// mountDebug exposes /debug/pprof/* for diagnosing a wedged daemon (the
// desktop watchdog captures goroutine?debug=2 before a respawn). It is a local
// operator tool, never a paired client's, so any request over TLS is refused.
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
			// Index serves both the listing at /debug/pprof/ and every named
			// profile (goroutine, heap, allocs, block, mutex, threadcreate);
			// it resolves the profile from the full /debug/pprof/<name> path.
			pprof.Index(c.Writer, c.Request)
		}
	}
	router.GET("/debug/pprof/*rest", handler)
	router.POST("/debug/pprof/*rest", handler)
}
