// Package home mounts the project-level home workspace routes under
// /v0/projects/:projectId/home. The home workspace has files, review threads,
// terminals and agentic chats, but no git: it is the project root, not a worktree.
package home

import (
	"github.com/gin-gonic/gin"

	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	homehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	threadhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads/handlers"
)

// Register mounts all home routes under projectScoped (/v0/projects/:projectId).
// Thread and chat handlers are reused from the workspace-scoped surface, with
// RequireHomeWorkspace injecting the :wsId the home URLs lack.
func Register(
	projectScoped *gin.RouterGroup,
	workspaces homehandlers.HomeWorkspaces,
	files homehandlers.Files,
	termEng termhandlers.TerminalEngine,
	working homehandlers.WorkSignal,
	threadStore threadhandlers.ThreadStore,
	threadBroadcast threadhandlers.ThreadBroadcaster,
	agentChats chathandlers.ChatUsecase,
	agentTurns chathandlers.TurnUsecase,
	agentRunners chathandlers.RunnerUsecase,
	agentAnswers chathandlers.AnswerUsecase,
	agentProviders chathandlers.ProviderUsecase,
	agentFolders chathandlers.ChatTreeUsecase,
	// agentWorktrees/agentNodes let the home chat list mark its owner row, or the
	// client could not tell it from an ordinary home chat.
	agentWorktrees chathandlers.Worktrees,
	agentNodes chathandlers.Nodes,
) {
	ah := chathandlers.New(
		agentChats, agentTurns, agentRunners, agentAnswers, agentProviders,
		agentFolders,
	).WithWorktrees(agentWorktrees).WithNodes(agentNodes)
	// agentChats doubles as the ChatResolver; GET /home resolves its owner through
	// the same OwnerOf the home chat list uses.
	h := homehandlers.New(workspaces, files, working).
		WithChats(agentChats).WithOwners(ah)
	th := threadhandlers.New(threadStore, threadBroadcast)
	home := projectScoped.Group("/home")

	home.GET("", h.Get)

	home.GET("/files/tree", h.FileTree)
	home.POST("/files", h.CreateFile)
	home.POST("/files/copy", h.CopyFile)
	home.PATCH("/files", h.RenameFile)
	home.DELETE("/files", h.DeleteFile)

	// Review threads' live changes ride the home workspace's ws: scope.
	home.GET("/threads", h.RequireHomeWorkspace, th.List)
	home.POST("/threads", h.RequireHomeWorkspace, th.OpenThread)
	home.GET("/threads/:threadId", h.RequireHomeWorkspace, th.Detail)
	home.PATCH("/threads/:threadId", h.RequireHomeWorkspace, th.SetResolved)
	home.DELETE("/threads/:threadId", h.RequireHomeWorkspace, th.DeleteThread)
	home.POST("/threads/:threadId/replies", h.RequireHomeWorkspace, th.Reply)
	home.PATCH("/threads/:threadId/messages/:messageId", h.RequireHomeWorkspace, th.EditMessage)
	home.DELETE("/threads/:threadId/messages/:messageId", h.RequireHomeWorkspace, th.DeleteMessage)

	// Terminals reuse the chat-scoped terminal handlers; RequireHomeTerminalScope
	// makes the home workspace the sessions' owner and starting directory.
	terms := termhandlers.New(termEng, nil)
	home.GET("/terminals", h.RequireHomeTerminalScope, terms.ListSessions)
	home.POST("/terminals", h.RequireHomeTerminalScope, terms.CreateSession)
	home.DELETE("/terminals/:sessionId", h.RequireHomeTerminalScope, terms.KillSession)
	home.GET("/terminals/:sessionId/ws", terms.WS)

	registerAgent(home, h, ah)
}

// registerAgent mounts the agent surface under /home. It duplicates the
// workspace group's chat table; TestHomeMountsEveryAgentRoute keeps them in step.
func registerAgent(
	home *gin.RouterGroup,
	h *homehandlers.Handlers,
	ah *chathandlers.Handlers,
) {
	// Chats must work for every workspace kind. Mounting them here also makes the
	// CLI's hooks, handoff callback and MCP relay reachable for a repo-less home scope.
	home.POST("/chats", h.RequireHomeWorkspace, ah.Create)
	home.GET("/chats", h.RequireHomeWorkspace, ah.List)
	home.GET("/chats/:id", h.RequireHomeWorkspace, ah.Get)
	home.POST("/chats/:id/prompts", h.RequireHomeWorkspace, ah.SubmitPrompt)
	home.POST("/chats/:id/attachments", h.RequireHomeWorkspace, ah.UploadAttachment)
	home.GET("/chats/:id/attachments/:file", h.RequireHomeWorkspace, ah.Attachment)
	home.HEAD("/chats/:id/attachments/:file", h.RequireHomeWorkspace, ah.Attachment)
	home.POST("/chats/:id/choices/:choiceId/answer", h.RequireHomeWorkspace, ah.AnswerChoice)
	home.POST("/chats/:id/terminal-wait/answer", h.RequireHomeWorkspace, ah.AnswerTerminalWait)
	home.PUT("/chats/:id/permission-level", h.RequireHomeWorkspace, ah.SetChatPermissionLevel)
	home.GET("/chats/:id/telemetry", h.RequireHomeWorkspace, ah.Telemetry)
	home.GET("/chats/:id/slash-catalog", h.RequireHomeWorkspace, ah.SlashCatalog)
	home.POST("/chats/:id/switch", h.RequireHomeWorkspace, ah.Switch)
	home.POST("/chats/:id/resume", h.RequireHomeWorkspace, ah.Resume)
	home.POST("/chats/:id/compact", h.RequireHomeWorkspace, ah.Compact)
	home.POST("/chats/:id/stop", h.RequireHomeWorkspace, ah.Stop)
	home.POST("/chats/:id/prompts/:requestId/retract", h.RequireHomeWorkspace, ah.Retract)
	home.POST("/chats/:id/switch-to-terminal", h.RequireHomeWorkspace, ah.SwitchToTerminal)
	home.POST("/chats/:id/switch-to-native", h.RequireHomeWorkspace, ah.SwitchToNative)
	home.POST("/chats/:id/rename", h.RequireHomeWorkspace, ah.Rename)
	home.PATCH("/chats/:id/selection", h.RequireHomeWorkspace, ah.SetSelection)
	home.GET("/chats/:id/handoff", h.RequireHomeWorkspace, ah.Handoff)
	home.PATCH("/chats/:id/placement", h.RequireHomeWorkspace, ah.PlaceChat)
	home.POST("/chats/:id/move/cancel", h.RequireHomeWorkspace, ah.CancelMove)
	home.POST("/chats/:id/branch-out", h.RequireHomeWorkspace, ah.BranchOut)
	home.DELETE("/chats/:id", h.RequireHomeWorkspace, ah.Delete)
	// Chat folders: the project home accumulates the most chats, so it most needs them.
	home.GET("/chats/folders", h.RequireHomeWorkspace, ah.ListFolders)
	home.POST("/chats/folders", h.RequireHomeWorkspace, ah.CreateFolder)
	home.PATCH("/chats/folders/:folderId", h.RequireHomeWorkspace, ah.PatchFolder)
	home.DELETE("/chats/folders/:folderId", h.RequireHomeWorkspace, ah.DeleteFolder)
	home.POST("/chats/runners/:segid/mcp", h.RequireHomeWorkspace, ah.MCP)
	home.POST("/chats/runners/:segid/hooks/:provider/:event", h.RequireHomeWorkspace, ah.HTTPHook)
	home.GET("/chats/providers", h.RequireHomeWorkspace, ah.Providers)
}
