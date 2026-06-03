// Package mcp exposes the MCP coordination layer between AI agents and
// Crowbar's domain repositories. It implements the MCPRepository interface,
// which mounts an HTTP handler group on the Gin router for all MCP traffic.
package mcp

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	mcpinternal "github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal"
	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/server"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// MCPRepository coordinates AI agent tool calls against domain repositories.
type MCPRepository interface {
	// Mount registers the MCP HTTP routes on the provided Gin router group.
	Mount(rg *gin.RouterGroup)
}

// mcpRepository is the unexported implementation.
type mcpRepository struct {
	srv *server.Server
}

// New constructs an MCPRepository, wiring the internal MCP server with the
// provided domain dependencies.
func New(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	kanbanRepo mcpinternal.KanbanItemAccessor,
	threadRepo mcpinternal.ReviewThreadAccessor,
	chatHub hub.ChatHub,
	flowLoader flow.Loader,
) MCPRepository {
	srv := server.New(agentRunRepo, taskRepo, kanbanRepo, threadRepo, chatHub, flowLoader)
	return &mcpRepository{srv: srv}
}

// Mount registers all MCP routes on the given Gin router group.
func (r *mcpRepository) Mount(rg *gin.RouterGroup) {
	r.srv.Mount(rg)
}
