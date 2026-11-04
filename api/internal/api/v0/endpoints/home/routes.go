// Package home mounts the project-level home workspace routes under
// /v0/projects/:projectId/home. The home workspace has files, a live
// file-change stream, review threads, terminals, and agentic chats — but no git
// operations.
package home

import (
	"github.com/gin-gonic/gin"

	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	homehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	threadhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads/handlers"
)

// Register mounts all home routes under projectScoped
// (/v0/projects/:projectId).
//
// Files and review threads are reused from the workspace-scoped surface: the
// home group has no :wsId path segment, so RequireHomeWorkspace resolves the
// project's home workspace and injects :wsId before each reused handler runs.
// That lets the thread REST handlers and the agent chat REST routes resolve the
// home workspace by id. Git is intentionally
// absent — the home workspace is the project root, not a per-workspace git
// worktree.
//
// working is the daemon's working-overlay read seam (the same one the workspaces
// handlers stamp their list/detail from): GET /home is the ONLY read path for the
// home workspace's DTO, so without it a home read taken while an agent chat is
// mid-turn would report working=false and contradict the home workspace's own
// stream entity.
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
	// agentWorktrees/agentNodes give the home chat list the same worktree
	// enrichment a repo's has: the home workspace rides no repo and has no
	// git surface, but its OWNER row still has to say it owns the home
	// worktree, or the client cannot tell it from an ordinary home chat.
	agentWorktrees chathandlers.Worktrees,
	agentNodes chathandlers.Nodes,
) {
	// agentChats already satisfies homehandlers.ChatResolver (ListChatsByWorkspace) —
	// no new dependency to thread through Register, only to wire in here.
	ah := chathandlers.New(
		agentChats, agentTurns, agentRunners, agentAnswers, agentProviders,
		agentFolders,
	).WithWorktrees(agentWorktrees).WithNodes(agentNodes)
	// GET /home resolves its owner through the SAME OwnerOf the home chat list
	// does.
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

	// Review threads are a home capability; their live changes ride the home
	// workspace's ws: scope. Every route resolves the home workspace via
	// RequireHomeWorkspace.
	home.GET("/threads", h.RequireHomeWorkspace, th.List)
	home.POST("/threads", h.RequireHomeWorkspace, th.OpenThread)
	home.GET("/threads/:threadId", h.RequireHomeWorkspace, th.Detail)
	home.PATCH("/threads/:threadId", h.RequireHomeWorkspace, th.SetResolved)
	home.DELETE("/threads/:threadId", h.RequireHomeWorkspace, th.DeleteThread)
	home.POST("/threads/:threadId/replies", h.RequireHomeWorkspace, th.Reply)
	home.PATCH("/threads/:threadId/messages/:messageId", h.RequireHomeWorkspace, th.EditMessage)
	home.DELETE("/threads/:threadId/messages/:messageId", h.RequireHomeWorkspace, th.DeleteMessage)

	// Terminals are served by the ONE terminal handler set the chat-scoped group uses
	// (same list DTO, same PTY WebSocket). RequireHomeTerminalScope makes the home
	// workspace the sessions' owner and their starting directory.
	terms := termhandlers.New(termEng, nil)
	home.GET("/terminals", h.RequireHomeTerminalScope, terms.ListSessions)
	home.POST("/terminals", h.RequireHomeTerminalScope, terms.CreateSession)
	home.DELETE("/terminals/:sessionId", h.RequireHomeTerminalScope, terms.KillSession)
	home.GET("/terminals/:sessionId/ws", terms.WS)

	registerAgent(home, h, ah)
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
) {
	// Agentic chats are a home capability too (00 agentic-engine spec: chats must
	// work for EVERY workspace kind, project-home included). The workspace-scoped
	// surface (agent.Register) mounts the SAME agent handler set under
	// .../workspaces/:wsId; here it is re-mounted under the home group with no
	// :wsId path segment, so RequireHomeWorkspace resolves the project's home
	// workspace and injects :wsId before each handler. This also makes the
	// CLI's hooks, its handoff callback and the agent's own MCP relay
	// reachable for a project-home workspace, whose repo-less scope resolves to
	// this /home/agent mount (see cmd/crowbar/scope.go's home branch).
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
	home.POST("/chats/:id/promote", h.RequireHomeWorkspace, ah.Promote)
	home.DELETE("/chats/:id", h.RequireHomeWorkspace, ah.Delete)
	// Chat FOLDERS, mounted here for the reason the chats above are: the project
	// home is the surface that accumulates the most chats, so it is the one that
	// most needs somewhere to put them. Mounting them only on the workspace group
	// would have left the home with a Chats panel that cannot be organised at all.
	home.GET("/chats/folders", h.RequireHomeWorkspace, ah.ListFolders)
	home.POST("/chats/folders", h.RequireHomeWorkspace, ah.CreateFolder)
	home.PATCH("/chats/folders/:folderId", h.RequireHomeWorkspace, ah.PatchFolder)
	home.DELETE("/chats/folders/:folderId", h.RequireHomeWorkspace, ah.DeleteFolder)
	home.POST("/chats/runners/:segid/mcp", h.RequireHomeWorkspace, ah.MCP)
	home.POST("/chats/runners/:segid/hooks/:provider/:event", h.RequireHomeWorkspace, ah.HTTPHook)
	home.GET("/chats/providers", h.RequireHomeWorkspace, ah.Providers)
}
