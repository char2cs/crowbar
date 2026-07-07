// Package handlers holds the gin handlers backing the health endpoint.
package handlers

import (
	"os"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// Check is the GET /health handler. It reports liveness by returning the
// daemon status, build version, and pid inside the standard query envelope:
// {"success":true,"data":{"status":"ok","version":"<v>","pid":<n>}}.
//
// The pid is load-bearing: the desktop supervisor signals the daemon
// (SIGTERM on quit, SIGQUIT/SIGKILL on wedge) by this self-reported pid,
// because asking the tauri-shell child handle for it deadlocks against the
// plugin's blocking wait thread.
func Check(
	c *gin.Context,
) {
	libs.WriteQueryOK(
		c,
		gin.H{
			"status":  "ok",
			"version": metadata.GetVersion(),
			"pid":     os.Getpid(),
		},
	)
}
