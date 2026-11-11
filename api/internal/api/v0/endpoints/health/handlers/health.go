// Package handlers holds the gin handlers backing the health endpoint.
package handlers

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// DeadLetters counts the hooks the chat ingress refused and kept.
type DeadLetters interface {
	DeadLetterCount(ctx context.Context) (int, error)
}

// Health serves the health endpoint.
type Health struct {
	deadLetters DeadLetters
}

// New builds the health handler over the chat ingress's dead-letter count.
func New(deadLetters DeadLetters) *Health {
	return &Health{deadLetters: deadLetters}
}

// Check is the GET /health liveness handler: status, version, pid and the refused-chat-hook count.
// The desktop supervisor signals the daemon by this self-reported pid because asking the tauri-shell
// child handle deadlocks against its wait thread. An unreadable count is omitted, never a failure.
func (h *Health) Check(
	c *gin.Context,
) {
	data := gin.H{
		"status":  "ok",
		"version": metadata.GetVersion(),
		"pid":     os.Getpid(),
	}
	count, err := h.deadLetters.DeadLetterCount(c.Request.Context())
	if err != nil {
		slog.WarnContext(c.Request.Context(), "health: count chat dead letters", "err", err)
	} else {
		data["chatDeadLetters"] = count
	}
	libs.WriteQueryOK(c, data)
}
