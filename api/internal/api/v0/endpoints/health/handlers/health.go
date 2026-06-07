// Package handlers holds the gin handlers backing the health endpoint.
package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// Check is the GET /health handler. It reports liveness by returning the
// daemon status and build version inside the standard query envelope:
// {"success":true,"data":{"status":"ok","version":"<v>"}}.
func Check(
	c *gin.Context,
) {
	libs.WriteQueryOK(
		c,
		gin.H{
			"status":  "ok",
			"version": metadata.GetVersion(),
		},
	)
}
