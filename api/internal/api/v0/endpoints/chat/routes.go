// Package agent mounts the repo-scoped .../repos/:repoId/chats REST and
// WebSocket routes: agentic-chat lifecycle CRUD, the vendor-CLI hook ingestion
// endpoint, and the agent-chat lifecycle WebSocket (00 agentic-engine spec;
// Task 17 rescoped the surface off the workspace group, since a chat's
// workspace is optional and mutable and a URL that names one goes stale).
package chat

import (
	"github.com/gin-gonic/gin"

	agenthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
)

// Register mounts the agent REST routes and the agent-chat lifecycle WebSocket
// upgrade route on the supplied repo-scoped router group (repoScoped, i.e.
// .../projects/:projectId/repos/:repoId), mirroring how files.Register and
// search.Register already sit on this group. A chat is no longer addressed
// through a workspace (model spec §5.1), so Create/List/Get/Switch/Rename/
// Handoff/Delete take only the chat's own :id; a handler that genuinely needs
// a specific chat's workspace resolves it from the chat itself (GetChat)
// rather than from the URL. The WS route lands in the SAME group, giving the
// resulting route .../repos/:repoId/chats/ws.
//
// .../chats/runners/:segid/mcp is keyed by the RUNNER, not the chat, for two
// reasons. It resolves runnerID → runner → CurrentChatID at call time, so nothing
// an agent is told at spawn can go stale when a /clear or /resume moves its CLI
// between chats. And it is the transport for the agent's OWN tool calls, so its
// authority must come from the runner's per-boot token rather than from a URL the
// agent composes.
//
// It is the only runner-keyed write left: the agent's titling path was a sibling
// route the vendor CLI shelled out to, and it is gone — titling is a tool on this
// MCP surface now, so there is nothing for an agent to retype.
//
// The chat FOLDER routes mount on the same group and for the same reason: a
// folder is repo-scoped, it shares one sibling space with the chats above it,
// and .../chats/folders is where a client already looks for everything about
// this panel. They are re-mounted under the home group too (home.Register) — the
// project home accumulates more chats than any workspace and is precisely where
// folders are needed, so mounting them once would have left the surface that
// needs them most without them.
//
// .../chats/:id/choices/:choiceId/answer and the two .../chats/hooks/*
// leaves are the ANSWER CHANNEL, and they are three routes because they serve
// two different callers. The first is a human, deciding a question in the chat.
// The other two are the in-PTY relay: one parks it alive while the provider's
// gate stays open, the other is what it reports when the provider kills it
// because somebody decided at the terminal instead. They mount beside
// .../chats/hooks for the same reason it does — that is where a vendor CLI's
// callbacks already reach this daemon.
//
// .../chats/:id/placement is the chat half of the same gesture the folder
// PATCH serves. It is a separate route from the chat's own rename because it
// writes something different in kind: a chat's parent IS its context lineage, so
// this endpoint can turn a standalone chat into a thread of another and back.
//
// .../chats/:id/promote is the model spec's §4.2 verb: a bubble fills its empty
// workspace slot and keeps everything else about itself. It is a POST rather
// than a field on some PATCH because it is not a field write — it cuts a
// branch, adds a worktree and respawns the CLI in it — and because it is
// one-way: a worktree is never demoted, so there is no opposite value to send.
//
// .../chats/:id/delete-preview is DELETE's dry run: what an idle delete's
// confirm dialog names before the user commits to it. id may be a chat or a
// folder, and a folder's subtree can span more than one independent
// workspace, so the file count it returns is a real sum across every one of
// them rather than a single workspace's own already-known count (backend
// addendum spec §1).
//
// settingsRG is the top-level /v0 group. Provider PRIORITY + enable/disable is a
// GLOBAL user setting (per user/machine, not per workspace — the CLIs are
// machine-level), so its write route mounts there at /settings/chat/providers,
// outside the entity hierarchy — mirroring /settings/terminal/profiles. It is the
// counterpart of the repo-scoped enriched GET .../chats/providers above, and
// is mounted exactly once (the home group re-mounts the GET but never this write).
// repos resolves :repoId for the one route that needs the repository itself:
// POST /chats with an `import` body, which adopts a branch that already exists
// in that repo (spec §4.1 — Create and Import are ONE route with a
// WorktreeSpec, not two). Every other route below is unaffected by it.
func Register(
	repoScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	chats agenthandlers.ChatUsecase,
	turns agenthandlers.TurnUsecase,
	runners agenthandlers.RunnerUsecase,
	answers agenthandlers.AnswerUsecase,
	providers agenthandlers.ProviderUsecase,
	folders agenthandlers.ChatTreeUsecase,
	repos agenthandlers.Repos,
	broadcastFolder func(folderID, workspaceID, kind string),
	wsHandle gin.HandlerFunc,
) {
	h := agenthandlers.New(chats, turns, runners, answers, providers, folders, broadcastFolder).
		WithRepos(repos)

	repoScoped.POST("/chats", h.Create)
	repoScoped.GET("/chats", h.List)
	repoScoped.GET("/chats/:id", h.Get)
	repoScoped.GET("/chats/:id/messages", h.Messages)
	repoScoped.POST("/chats/:id/prompts", h.SubmitPrompt)
	repoScoped.GET("/chats/:id/activity", h.Activity)
	repoScoped.GET("/chats/:id/activity/:toolId/payload", h.ToolPayload)
	repoScoped.GET("/chats/:id/choices", h.Choices)
	repoScoped.POST("/chats/:id/choices/:choiceId/answer", h.AnswerChoice)
	repoScoped.PUT("/chats/:id/permission-level", h.SetChatPermissionLevel)
	repoScoped.GET("/chats/:id/telemetry", h.Telemetry)
	repoScoped.GET("/chats/:id/slash-catalog", h.SlashCatalog)
	repoScoped.POST("/chats/:id/switch", h.Switch)
	repoScoped.POST("/chats/:id/resume", h.Resume)
	repoScoped.POST("/chats/:id/compact", h.Compact)
	repoScoped.POST("/chats/:id/stop", h.Stop)
	repoScoped.POST("/chats/:id/switch-to-terminal", h.SwitchToTerminal)
	repoScoped.POST("/chats/:id/switch-to-native", h.SwitchToNative)
	repoScoped.POST("/chats/:id/rename", h.Rename)
	repoScoped.PATCH("/chats/:id/selection", h.SetSelection)
	repoScoped.GET("/chats/:id/handoff", h.Handoff)
	repoScoped.PATCH("/chats/:id/placement", h.PlaceChat)
	repoScoped.POST("/chats/:id/promote", h.Promote)
	repoScoped.DELETE("/chats/:id", h.Delete)
	repoScoped.GET("/chats/:id/delete-preview", h.DeletePreview)
	repoScoped.GET("/chats/folders", h.ListFolders)
	repoScoped.POST("/chats/folders", h.CreateFolder)
	repoScoped.PATCH("/chats/folders/:folderId", h.PatchFolder)
	repoScoped.DELETE("/chats/folders/:folderId", h.DeleteFolder)
	repoScoped.POST("/chats/runners/:segid/mcp", h.MCP)
	repoScoped.POST("/chats/hooks", h.Hooks)
	repoScoped.POST("/chats/hooks/await", h.AwaitHookAnswer)
	repoScoped.POST("/chats/hooks/abandon", h.AbandonHookAnswer)
	repoScoped.GET("/chats/providers", h.Providers)
	repoScoped.GET("/chats/ws", wsHandle)

	settingsRG.PUT("/settings/chat/providers", h.UpdateProviderPreferences)
	settingsRG.GET("/settings/chat/permission-level", h.GetDefaultPermissionLevel)
	settingsRG.PUT("/settings/chat/permission-level", h.PutDefaultPermissionLevel)
}
