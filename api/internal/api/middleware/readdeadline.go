package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// PreAuthReadDeadline bounds how long a request arriving over TLS may take to
// deliver its body, including the drain net/http does after an early answer.
// Until a guard verifies the caller (LiftReadDeadline) the peer is anonymous,
// and an unbounded read lets it pin a connection by trickling bytes. Plaintext
// is the local transport, whose trust boundary is the OS, so it is left alone.
func PreAuthReadDeadline(
	timeout time.Duration,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.TLS == nil {
			c.Next()
			return
		}
		if err := setReadDeadline(c.Writer, time.Now().Add(timeout)); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	}
}

// LiftReadDeadline clears PreAuthReadDeadline's bound once the caller is
// authenticated, so a long upload, stream or hijacked WebSocket is not cut.
func LiftReadDeadline(
	c *gin.Context,
) error {
	return setReadDeadline(c.Writer, time.Time{})
}

// setReadDeadline reaches the connection behind w. A writer with none (an
// in-memory recorder) has nothing to bound, so that is not an error.
func setReadDeadline(
	w http.ResponseWriter,
	deadline time.Time,
) error {
	err := http.NewResponseController(w).SetReadDeadline(deadline)
	if err == nil || errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return fmt.Errorf("middleware: set read deadline: %w", err)
}
