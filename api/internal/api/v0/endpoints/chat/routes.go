package chat

import (
	"github.com/gin-gonic/gin"

	agenthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
)

func Register(
	repoScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	chats agenthandlers.ChatUsecase,
	turns agenthandlers.TurnUsecase,
	runners agenthandlers.RunnerUsecase,
	answers agenthandlers.AnswerUsecase,
	providers agenthandlers.ProviderUsecase,
	folders agenthandlers.ChatTreeUsecase,
	repos agenthandlers.Repos,
	worktrees agenthandlers.Worktrees,
	nodes agenthandlers.Nodes,
) {
	h := agenthandlers.New(chats, turns, runners, answers, providers, folders).
		WithRepos(repos).
		WithWorktrees(worktrees).
		WithNodes(nodes)

	chatScoped.POST("/kill", h.Kill)
	repoScoped.POST("/chats", h.Create)
	repoScoped.GET("/chats", h.List)
	repoScoped.GET("/workspaces", h.Workspaces)
	repoScoped.GET("/chats/:id", h.Get)
	repoScoped.POST("/chats/:id/prompts", h.SubmitPrompt)
	repoScoped.POST("/chats/:id/attachments", h.UploadAttachment)
	repoScoped.POST("/chats/:id/attachments/uploads", h.BeginAttachmentUpload)
	repoScoped.PUT("/chats/:id/attachments/uploads/:uploadId", h.AppendAttachmentUpload)
	repoScoped.POST("/chats/:id/attachments/uploads/:uploadId/complete", h.CompleteAttachmentUpload)
	repoScoped.DELETE("/chats/:id/attachments/uploads/:uploadId", h.AbortAttachmentUpload)
	repoScoped.GET("/chats/:id/attachments/:file", h.Attachment)
	repoScoped.HEAD("/chats/:id/attachments/:file", h.Attachment)
	repoScoped.POST("/chats/:id/choices/:choiceId/answer", h.AnswerChoice)
	repoScoped.POST("/chats/:id/terminal-wait/answer", h.AnswerTerminalWait)
	repoScoped.PUT("/chats/:id/permission-level", h.SetChatPermissionLevel)
	repoScoped.GET("/chats/:id/telemetry", h.Telemetry)
	repoScoped.GET("/chats/:id/slash-catalog", h.SlashCatalog)
	repoScoped.POST("/chats/:id/switch", h.Switch)
	repoScoped.POST("/chats/:id/resume", h.Resume)
	repoScoped.POST("/chats/:id/compact", h.Compact)
	repoScoped.POST("/chats/:id/shells/:shellId/stop", h.StopShell)
	repoScoped.POST("/chats/:id/stop", h.Stop)
	repoScoped.POST("/chats/:id/prompts/:requestId/retract", h.Retract)
	repoScoped.POST("/chats/:id/switch-to-terminal", h.SwitchToTerminal)
	repoScoped.POST("/chats/:id/switch-to-native", h.SwitchToNative)
	repoScoped.POST("/chats/:id/rename", h.Rename)
	repoScoped.PATCH("/chats/:id/selection", h.SetSelection)
	repoScoped.GET("/chats/:id/handoff", h.Handoff)
	repoScoped.PATCH("/chats/:id/placement", h.PlaceChat)
	repoScoped.POST("/chats/:id/move/cancel", h.CancelMove)
	repoScoped.POST("/chats/:id/branch-out", h.BranchOut)
	repoScoped.DELETE("/chats/:id", h.Delete)
	repoScoped.GET("/chats/folders", h.ListFolders)
	repoScoped.POST("/chats/folders", h.CreateFolder)
	repoScoped.PATCH("/chats/folders/:folderId", h.PatchFolder)
	repoScoped.DELETE("/chats/folders/:folderId", h.DeleteFolder)
	repoScoped.POST("/chats/runners/:segid/mcp", h.MCP)
	repoScoped.POST("/chats/runners/:segid/hooks/:provider/:event", h.HTTPHook)
	repoScoped.GET("/chats/providers", h.Providers)

	settingsRG.POST("/relays/:relay/mcp", h.RelayMCP)
	settingsRG.PUT("/settings/chat/providers", h.UpdateProviderPreferences)
	settingsRG.GET("/settings/chat/providers", h.Providers)
	settingsRG.GET("/settings/chat/descriptors", h.DescriptorReports)
	settingsRG.POST("/settings/chat/descriptors", h.UploadDescriptor)
	settingsRG.GET("/settings/chat/permission-level", h.GetDefaultPermissionLevel)
	settingsRG.PUT("/settings/chat/permission-level", h.PutDefaultPermissionLevel)
	settingsRG.GET("/settings/chat/model-manifest-fetch", h.GetModelManifestFetchEnabled)
	settingsRG.PUT("/settings/chat/model-manifest-fetch", h.PutModelManifestFetchEnabled)
}
