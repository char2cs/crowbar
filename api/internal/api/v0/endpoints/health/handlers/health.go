package handlers

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

type DeadLetters interface {
	DeadLetterCount(ctx context.Context) (int, error)
}

type Health struct {
	deadLetters DeadLetters
}

func New(deadLetters DeadLetters) *Health {
	return &Health{deadLetters: deadLetters}
}

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
