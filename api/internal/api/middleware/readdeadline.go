package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

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

func LiftReadDeadline(
	c *gin.Context,
) error {
	return setReadDeadline(c.Writer, time.Time{})
}

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
