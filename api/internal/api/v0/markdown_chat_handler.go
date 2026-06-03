package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

// MarkdownChat serves the cold-start turn history for a markdown conversation.
func MarkdownChat(c *gin.Context) {
	c.JSON(http.StatusOK, []fixtures.MarkdownTurn{
		{
			ID:         "t1",
			Role:       "user",
			Content:    "How does this feature work?",
			Timestamp:  "2026-06-01T00:00:00Z",
			AuthorName: "You",
			Widgets:    []any{},
		},
		{
			ID:         "t2",
			Role:       "agent",
			Content:    "Here's a walkthrough of how it works…",
			Timestamp:  "2026-06-01T00:00:05Z",
			AuthorName: "Claude",
			Model:      "Opus 4.8",
			Widgets:    []any{},
		},
	})
}
