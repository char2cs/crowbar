package terminal

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/internal/handler"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
)

// TerminalHandler handles PTY WebSocket connections for a task's worktree.
type TerminalHandler interface {
	Handle(
		ctx    context.Context,
		c      *gin.Context,
		taskID string,
	)
}

// New constructs a TerminalHandler backed by the given Task repository.
func New(
	taskRepo repositories.Task,
) TerminalHandler {
	return handler.New(taskRepo)
}

// Register mounts the terminal WebSocket route on rg.
func Register(rg *gin.RouterGroup, h TerminalHandler) {
	rg.GET("/tasks/:id/terminal", func(c *gin.Context) {
		h.Handle(context.Background(), c, c.Param("id"))
	})
}
