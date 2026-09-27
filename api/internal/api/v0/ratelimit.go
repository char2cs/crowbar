package v0

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// newFixedWindowLimiter rejects requests past max within a single rolling
// window, reset (not decayed) once the window elapses. It is global, not
// per-caller: the pairing-redeem endpoint has no identity to key on before a
// caller is paired, and a global cap on wrong guesses is the point.
func newFixedWindowLimiter(max int, window time.Duration) gin.HandlerFunc {
	var (
		mu         sync.Mutex
		count      int
		windowEnds time.Time
	)
	return func(c *gin.Context) {
		mu.Lock()
		now := time.Now()
		if now.After(windowEnds) {
			count = 0
			windowEnds = now.Add(window)
		}
		count++
		over := count > max
		mu.Unlock()

		if over {
			libs.WriteErr(c, http.StatusTooManyRequests, "too many requests, try again shortly")
			c.Abort()
			return
		}
		c.Next()
	}
}
