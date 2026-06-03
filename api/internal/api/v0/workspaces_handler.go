package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type WorkspacesHandler struct {
	store *fixtures.Store
}

func NewWorkspacesHandler(store *fixtures.Store) *WorkspacesHandler {
	return &WorkspacesHandler{store: store}
}

func (h *WorkspacesHandler) Get(c *gin.Context) {
	ws, ok := h.store.GetWorkspace(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	c.JSON(http.StatusOK, ws)
}

func (h *WorkspacesHandler) Create(c *gin.Context) {
	var req struct {
		RepoID   string `json:"repoId" binding:"required"`
		Branch   string `json:"branch" binding:"required"`
		FlowName string `json:"flowName" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var flow fixtures.FlowDefinition
	for _, f := range h.store.Flows {
		if f.Name == req.FlowName {
			flow = f
			break
		}
	}
	initialState := ""
	if len(flow.States) > 0 {
		initialState = flow.States[0].Name
	}
	ws := fixtures.WorkspacePayload{
		ID:           uuid.New().String(),
		RepoID:       req.RepoID,
		Branch:       req.Branch,
		FlowName:     req.FlowName,
		CurrentState: initialState,
		Flow:         flow,
	}
	h.store.AddWorkspace(ws)
	c.JSON(http.StatusCreated, ws)
}
