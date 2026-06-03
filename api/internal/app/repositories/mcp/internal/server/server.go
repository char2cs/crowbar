// Package server sets up the mcp-go MCPServer, registers all crowbar tools,
// and mounts the resulting HTTP handler onto a Gin router group.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	mcpinternal "github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal"
	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/tools"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// Server wraps the mcp-go StreamableHTTPServer and exposes Mount for Gin
// integration.
type Server struct {
	handler http.Handler
}

// New constructs the MCP server, registers all 8 crowbar tools, and returns a
// Server ready for mounting.
func New(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	kanbanRepo mcpinternal.KanbanItemAccessor,
	threadRepo mcpinternal.ReviewThreadAccessor,
	chatHub hub.ChatHub,
	flowLoader flow.Loader,
) *Server {
	mcpSrv := mcpserver.NewMCPServer(
		"crowbar",
		"0.1.0",
		mcpserver.WithToolCapabilities(false),
	)

	// Register all 8 tools.
	mcpSrv.AddTool(tools.NewSignalTool(), tools.NewSignalHandler(
		agentRunRepo, taskRepo, chatHub, flowLoader,
	))
	mcpSrv.AddTool(tools.NewCreateItemTool(), tools.NewCreateItemHandler(
		agentRunRepo, taskRepo, kanbanRepo, flowLoader,
	))
	mcpSrv.AddTool(tools.NewUpdateItemStatusTool(), tools.NewUpdateItemStatusHandler(
		agentRunRepo, taskRepo, kanbanRepo, flowLoader,
	))
	mcpSrv.AddTool(tools.NewGetItemsTool(), tools.NewGetItemsHandler(
		agentRunRepo, taskRepo, kanbanRepo, flowLoader,
	))
	mcpSrv.AddTool(tools.NewOpenThreadTool(), tools.NewOpenThreadHandler(
		agentRunRepo, taskRepo, threadRepo, flowLoader,
	))
	mcpSrv.AddTool(tools.NewReplyThreadTool(), tools.NewReplyThreadHandler(
		agentRunRepo, taskRepo, threadRepo, flowLoader,
	))
	mcpSrv.AddTool(tools.NewGetThreadsTool(), tools.NewGetThreadsHandler(
		agentRunRepo, taskRepo, threadRepo, flowLoader,
	))
	mcpSrv.AddTool(tools.NewResolveThreadTool(), tools.NewResolveThreadHandler(
		agentRunRepo, taskRepo, threadRepo, flowLoader,
	))

	// Wrap in the Streamable HTTP transport, injecting the token from the
	// X-Agent-Run-Token header into every request's context.
	streamable := mcpserver.NewStreamableHTTPServer(
		mcpSrv,
		mcpserver.WithStateLess(true),
		mcpserver.WithHTTPContextFunc(tools.TokenFromRequest),
	)

	return &Server{handler: streamable}
}

// Mount registers the MCP handler on every path under the provided Gin router
// group. Gin's Any method is used so the mcp-go library can serve both POST
// (JSON-RPC) and GET (SSE streaming) requests on the same path.
func (s *Server) Mount(rg *gin.RouterGroup) {
	h := s.handler
	ginHandler := func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
	rg.Any("", ginHandler)
	rg.Any("/*path", ginHandler)
}

