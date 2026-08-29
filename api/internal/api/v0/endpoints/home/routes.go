// Package home mounts the project-level home workspace routes under
// /v0/projects/:projectId/home. The home workspace has files, a live
// file-change stream, review threads, terminals, and agentic chats — but no git
// operations.
package home

import (
	"github.com/gin-gonic/gin"

	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	homehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
	threadhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads/handlers"
)

// Register mounts all home routes under projectScoped
// (/v0/projects/:projectId).
//
// Files and review threads are reused from the workspace-scoped surface: the
// home group has no :wsId path segment, so RequireHomeWorkspace resolves the
// project's home workspace and injects :wsId before each reused handler runs.
// That lets the file-change WS handler (filesWS), the dual-served thread list
// WS (threadsWS), the thread REST handlers, and the agent chat REST + WS
// (the agent concerns/agentWS) resolve the home workspace by id. Git is intentionally
// absent — the home workspace is the project root, not a per-workspace git
// worktree.
//
// working is the daemon's working-overlay read seam (the same one the workspaces
// handlers stamp their list/detail from): GET /home is the ONLY read path for the
// home workspace's DTO, so without it a home read taken while an agent chat is
// mid-turn would report working=false and contradict the home workspace's own WS
// frames.
func Register(
	projectScoped *gin.RouterGroup,
	workspaces homehandlers.HomeWorkspaces,
	projects homehandlers.ProjectReader,
	files homehandlers.Files,
	termEng homehandlers.TerminalEngine,
	working homehandlers.WorkSignal,
	filesWS gin.HandlerFunc,
	threadStore threadhandlers.ThreadStore,
	threadBroadcast threadhandlers.ThreadBroadcaster,
	threadsWS gin.HandlerFunc,
	agentChats chathandlers.ChatUsecase,
	agentTurns chathandlers.TurnUsecase,
	agentRunners chathandlers.RunnerUsecase,
	agentAnswers chathandlers.AnswerUsecase,
	agentProviders chathandlers.ProviderUsecase,
	agentFolders chathandlers.ChatTreeUsecase,
	agentBroadcastFolder func(folderID, workspaceID, kind string),
	agentWS gin.HandlerFunc,
	dispatch func(rest, wsHandler gin.HandlerFunc) gin.HandlerFunc,
) {
	h := homehandlers.New(workspaces, projects, files, termEng, working)
	th := threadhandlers.New(threadStore, threadBroadcast)
	ah := chathandlers.New(
		agentChats, agentTurns, agentRunners, agentAnswers, agentProviders,
		agentFolders, agentBroadcastFolder,
	)
	home := projectScoped.Group("/home")

	home.GET("", h.Get)

	home.GET("/files/tree", h.FileTree)
	home.GET("/files/content", h.FileContent)
	home.PUT("/files/content", h.SaveFileContent)
	home.POST("/files", h.CreateFile)
	home.POST("/files/copy", h.CopyFile)
	home.PATCH("/files", h.RenameFile)
	home.DELETE("/files", h.DeleteFile)
	// Live file-change stream for the home workspace, mirroring the workspace-
	// scoped .../files/ws leaf. RequireHomeWorkspace injects :wsId so the files
	// broadcaster's wsId filter and the watcher lifecycle scope to the home
	// workspace (rooted at the project path).
	home.GET("/files/ws", h.RequireHomeWorkspace, filesWS)

	// Review threads are a home capability. The list route is dual-served (a
	// plain GET lists threads; a WebSocket upgrade subscribes the thread topic);
	// the rest are REST mutations. Every route resolves the home workspace via
	// RequireHomeWorkspace.
	home.GET("/threads", h.RequireHomeWorkspace, dispatch(th.List, threadsWS))
	home.POST("/threads", h.RequireHomeWorkspace, th.OpenThread)
	home.GET("/threads/:threadId", h.RequireHomeWorkspace, th.Detail)
	home.PATCH("/threads/:threadId", h.RequireHomeWorkspace, th.SetResolved)
	home.DELETE("/threads/:threadId", h.RequireHomeWorkspace, th.DeleteThread)
	home.POST("/threads/:threadId/replies", h.RequireHomeWorkspace, th.Reply)
	home.PATCH("/threads/:threadId/messages/:messageId", h.RequireHomeWorkspace, th.EditMessage)
	home.DELETE("/threads/:threadId/messages/:messageId", h.RequireHomeWorkspace, th.DeleteMessage)

	home.GET("/terminals", h.ListTerminals)
	home.POST("/terminals", h.CreateTerminal)
	home.DELETE("/terminals/:sessionId", h.KillTerminal)
	home.GET("/terminals/:sessionId/ws", h.TerminalWS)

	registerAgent(home, h, ah, agentWS)
}

// registerAgent mounts the agent surface under /home.
//
// It is a function of its own for the reason its comment gives: it is a SECOND
// copy of the workspace group's table, kept in step by
// TestHomeMountsEveryAgentRoute, and a table long enough to need scrolling past
// is one people stop reading. Register stays a list of capabilities; this is one
// of them.
func registerAgent(
	home *gin.RouterGroup,
	h *homehandlers.Handlers,
	ah *chathandlers.Handlers,
	agentWS gin.HandlerFunc,
) {
	// Agentic chats are a home capability too (00 agentic-engine spec: chats must
	// work for EVERY workspace kind, project-home included). The workspace-scoped
	// surface (agent.Register) mounts the SAME agent handler set under
	// .../workspaces/:wsId; here it is re-mounted under the home group with no
	// :wsId path segment, so RequireHomeWorkspace resolves the project's home
	// workspace and injects :wsId before each handler — and before the lifecycle
	// WS (agentWS), whose agentChatDef filter keys on that injected :wsId so a
	// home client sees exactly the home workspace's chats. This also makes the
	// in-PTY CLI callbacks (crowbar hook/handoff) and the agent's own MCP relay
	// reachable for a project-home workspace, whose repo-less scope resolves to
	// this /home/agent mount (see cmd/crowbar/scope.go's home branch).
	home.POST("/chats", h.RequireHomeWorkspace, ah.Create)
	home.GET("/chats", h.RequireHomeWorkspace, ah.List)
	home.GET("/chats/:id", h.RequireHomeWorkspace, ah.Get)
	home.GET("/chats/:id/messages", h.RequireHomeWorkspace, ah.Messages)
	home.POST("/chats/:id/prompts", h.RequireHomeWorkspace, ah.SubmitPrompt)
	home.GET("/chats/:id/activity", h.RequireHomeWorkspace, ah.Activity)
	home.GET("/chats/:id/activity/:toolId/payload", h.RequireHomeWorkspace, ah.ToolPayload)
	home.GET("/chats/:id/choices", h.RequireHomeWorkspace, ah.Choices)
	home.POST("/chats/:id/choices/:choiceId/answer", h.RequireHomeWorkspace, ah.AnswerChoice)
	home.PUT("/chats/:id/permission-level", h.RequireHomeWorkspace, ah.SetChatPermissionLevel)
	home.GET("/chats/:id/telemetry", h.RequireHomeWorkspace, ah.Telemetry)
	home.GET("/chats/:id/slash-catalog", h.RequireHomeWorkspace, ah.SlashCatalog)
	home.POST("/chats/:id/switch", h.RequireHomeWorkspace, ah.Switch)
	home.POST("/chats/:id/resume", h.RequireHomeWorkspace, ah.Resume)
	home.POST("/chats/:id/compact", h.RequireHomeWorkspace, ah.Compact)
	home.POST("/chats/:id/stop", h.RequireHomeWorkspace, ah.Stop)
	home.POST("/chats/:id/switch-to-terminal", h.RequireHomeWorkspace, ah.SwitchToTerminal)
	home.POST("/chats/:id/switch-to-native", h.RequireHomeWorkspace, ah.SwitchToNative)
	home.POST("/chats/:id/rename", h.RequireHomeWorkspace, ah.Rename)
	home.PATCH("/chats/:id/selection", h.RequireHomeWorkspace, ah.SetSelection)
	home.GET("/chats/:id/handoff", h.RequireHomeWorkspace, ah.Handoff)
	home.PATCH("/chats/:id/placement", h.RequireHomeWorkspace, ah.PlaceChat)
	home.POST("/chats/:id/promote", h.RequireHomeWorkspace, ah.Promote)
	home.DELETE("/chats/:id", h.RequireHomeWorkspace, ah.Delete)
	home.GET("/chats/:id/delete-preview", h.RequireHomeWorkspace, ah.DeletePreview)
	// Chat FOLDERS, mounted here for the reason the chats above are: the project
	// home is the surface that accumulates the most chats, so it is the one that
	// most needs somewhere to put them. Mounting them only on the workspace group
	// would have left the home with a Chats panel that cannot be organised at all.
	home.GET("/chats/folders", h.RequireHomeWorkspace, ah.ListFolders)
	home.POST("/chats/folders", h.RequireHomeWorkspace, ah.CreateFolder)
	home.PATCH("/chats/folders/:folderId", h.RequireHomeWorkspace, ah.PatchFolder)
	home.DELETE("/chats/folders/:folderId", h.RequireHomeWorkspace, ah.DeleteFolder)
	home.POST("/chats/runners/:segid/mcp", h.RequireHomeWorkspace, ah.MCP)
	home.POST("/chats/hooks", h.RequireHomeWorkspace, ah.Hooks)
	// The answer channel's relay half. It mounts here for exactly the reason the
	// hook ingress above does: a project-home workspace's repo-less scope resolves
	// to this /home/agent mount, so a CLI running there reaches these leaves and no
	// others (see cmd/crowbar/scope.go).
	home.POST("/chats/hooks/await", h.RequireHomeWorkspace, ah.AwaitHookAnswer)
	home.POST("/chats/hooks/abandon", h.RequireHomeWorkspace, ah.AbandonHookAnswer)
	home.GET("/chats/providers", h.RequireHomeWorkspace, ah.Providers)
	home.GET("/chats/ws", h.RequireHomeWorkspace, agentWS)
}
