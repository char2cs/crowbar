// Package chat mounts the repo-scoped .../repos/:repoId/chats REST routes. They hang off the repo, not a
// workspace, because a chat's workspace is optional and mutable, so a URL naming one would go stale.
package chat

import (
	"github.com/gin-gonic/gin"

	agenthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
)

// Register mounts the chat routes on repoScoped (.../repos/:repoId), the flat /v0/chats/:chatId
// group (chatScoped) and the top-level /v0 group (settingsRG). Chat routes take only the chat's :id;
// a handler needing the chat's workspace resolves it from the chat, never from the URL.
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

	// Flat so a client can end any chat's runner knowing only its id.
	chatScoped.POST("/kill", h.Kill)
	repoScoped.POST("/chats", h.Create)
	repoScoped.GET("/chats", h.List)
	// Every workspace row, chat or not: a repo's default checkout and a never-chatted-in locked
	// branch have no chat to derive them from.
	repoScoped.GET("/workspaces", h.Workspaces)
	repoScoped.GET("/chats/:id", h.Get)
	repoScoped.POST("/chats/:id/prompts", h.SubmitPrompt)
	repoScoped.POST("/chats/:id/attachments", h.UploadAttachment)
	repoScoped.POST("/chats/:id/attachments/uploads", h.BeginAttachmentUpload)
	repoScoped.PUT("/chats/:id/attachments/uploads/:uploadId", h.AppendAttachmentUpload)
	repoScoped.POST("/chats/:id/attachments/uploads/:uploadId/complete", h.CompleteAttachmentUpload)
	repoScoped.DELETE("/chats/:id/attachments/uploads/:uploadId", h.AbortAttachmentUpload)
	repoScoped.GET("/chats/:id/attachments/:file", h.Attachment)
	// The web file-card size label issues HEAD here; net/http's HEAD handling suffices.
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
	// Separate from rename: a chat's parent is its context lineage, so placement can make a chat a
	// thread of another and back.
	repoScoped.PATCH("/chats/:id/placement", h.PlaceChat)
	repoScoped.POST("/chats/:id/move/cancel", h.CancelMove)
	// Cuts a branch from the thread's folder at its own HEAD and moves the thread there, at its
	// turn boundary; one-way (a branch is never made a thread again).
	repoScoped.POST("/chats/:id/branch-out", h.BranchOut)
	repoScoped.DELETE("/chats/:id", h.Delete)
	// Folders share one sibling space with the repo's chats; home.Register re-mounts them too.
	repoScoped.GET("/chats/folders", h.ListFolders)
	repoScoped.POST("/chats/folders", h.CreateFolder)
	repoScoped.PATCH("/chats/folders/:folderId", h.PatchFolder)
	repoScoped.DELETE("/chats/folders/:folderId", h.DeleteFolder)
	// Runner-keyed, not chat-keyed: the runner's current chat is resolved per call so a /clear or
	// /resume never strands the agent, and authority comes from the runner's per-boot token.
	repoScoped.POST("/chats/runners/:segid/mcp", h.MCP)
	repoScoped.POST("/chats/runners/:segid/hooks/:provider/:event", h.HTTPHook)
	repoScoped.GET("/chats/providers", h.Providers)

	// Provider priority and enablement are machine-wide (the CLIs are), so they sit outside the
	// entity hierarchy and load with no active workspace.
	settingsRG.PUT("/settings/chat/providers", h.UpdateProviderPreferences)
	settingsRG.GET("/settings/chat/providers", h.Providers)
	settingsRG.GET("/settings/chat/descriptors", h.DescriptorReports)
	settingsRG.POST("/settings/chat/descriptors", h.UploadDescriptor)
	settingsRG.GET("/settings/chat/permission-level", h.GetDefaultPermissionLevel)
	settingsRG.PUT("/settings/chat/permission-level", h.PutDefaultPermissionLevel)
	settingsRG.GET("/settings/chat/model-manifest-fetch", h.GetModelManifestFetchEnabled)
	settingsRG.PUT("/settings/chat/model-manifest-fetch", h.PutModelManifestFetchEnabled)
}
