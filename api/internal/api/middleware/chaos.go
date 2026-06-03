package middleware

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func Chaos() gin.HandlerFunc {
	return func(c *gin.Context) {
		if d := c.GetHeader("X-Crowbar-Latency"); d != "" {
			if ms, err := strconv.Atoi(d); err == nil && ms > 0 {
				time.Sleep(time.Duration(ms) * time.Millisecond)
			}
		}
		if r := c.GetHeader("X-Crowbar-Error-Rate"); r != "" {
			if rate, err := strconv.ParseFloat(r, 64); err == nil && rand.Float64() < rate {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "chaos injection"})
				return
			}
		}
		c.Next()
	}
}
