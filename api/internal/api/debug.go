// debug.go mounts the Go runtime's pprof surface on the daemon router.
package api

import (
	"net/http/pprof"

	"github.com/gin-gonic/gin"
)

// mountDebug exposes /debug/pprof/* so a live-but-wedged daemon can be
// diagnosed from outside: the desktop watchdog captures
// /debug/pprof/goroutine?debug=2 into the daemon log before it kills and
// respawns an unresponsive daemon. The daemon serves a private unix socket
// only, so this surface is reachable exclusively by local processes that can
// already open that socket.
func mountDebug(
	router *gin.Engine,
) {
	handler := func(c *gin.Context) {
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
