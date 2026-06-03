package v0

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type ProjectsHandler struct{ store *fixtures.Store }

func NewProjectsHandler(store *fixtures.Store) *ProjectsHandler {
	return &ProjectsHandler{store: store}
}

func (h *ProjectsHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.ListProjects())
}

func (h *ProjectsHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p := fixtures.Project{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Path:         req.Path,
		LastActivity: time.Now(),
	}
	h.store.AddProject(p)
	c.JSON(http.StatusCreated, p)
}
