package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TerminalHandler struct{}

func NewTerminalHandler() *TerminalHandler { return &TerminalHandler{} }

func (h *TerminalHandler) CreateSession(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"sessionId": uuid.New().String()})
}
