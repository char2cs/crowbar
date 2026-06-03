package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
)

// NotFound writes a 404 JSON error response.
func NotFound(c *gin.Context) {
	c.JSON(404, gin.H{"error": "not found"})
}

// InternalError writes a 500 JSON error response.
func InternalError(c *gin.Context, err error) {
	c.JSON(500, gin.H{"error": err.Error()})
}

// HandleRepoError maps a repository error to the appropriate HTTP response.
// Returns true if an error was written (caller should return), false otherwise.
func HandleRepoError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, repositories.ErrNotFound) {
		c.JSON(404, gin.H{"error": "not found"})
		return true
	}
	c.JSON(500, gin.H{"error": err.Error()})
	return true
}
