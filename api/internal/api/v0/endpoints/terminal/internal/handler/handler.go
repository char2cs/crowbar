package handler

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Handler is the interface for PTY WebSocket connections.
type Handler interface {
	Handle(
		ctx    context.Context,
		c      *gin.Context,
		taskID string,
	)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type terminalHandler struct {
	taskRepo repositories.Task
}

// New constructs a Handler backed by the given Task repository.
func New(
	taskRepo repositories.Task,
) Handler {
	return &terminalHandler{taskRepo: taskRepo}
}

// Handle resolves the task worktree path, upgrades the HTTP connection to
// WebSocket, starts a PTY subprocess inside that worktree directory, and
// bridges PTY I/O with the WebSocket until either side closes.
func (h *terminalHandler) Handle(
	ctx    context.Context,
	c      *gin.Context,
	taskID string,
) {
	task, err := h.taskRepo.Get(ctx, taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}

	cmd := exec.Command(shell)
	cmd.Dir = task.WorktreePath

	ptmx, err := pty.Start(cmd)
	if err != nil {
		_ = conn.WriteMessage(
			websocket.TextMessage,
			[]byte("failed to start terminal: "+err.Error()+"\r\n"),
		)
		return
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = ptmx.Close()
	}()

	s := &session{conn: conn, ptmx: ptmx}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		s.pumpPTYToWS()
	}()

	go func() {
		defer wg.Done()
		s.pumpWSToPTY()
	}()

	wg.Wait()
}
