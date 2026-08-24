// Package agent mounts the workspace-scoped .../workspaces/:wsId/agent REST and
// WebSocket routes: agentic-chat lifecycle CRUD, the vendor-CLI hook ingestion
// endpoint, and the agent-chat lifecycle WebSocket (00 agentic-engine spec;
// Task 3 nested the surface under the workspace group).
package chat

import (
	"github.com/gin-gonic/gin"

	agenthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
)

// Register mounts the agent REST routes and the agent-chat lifecycle WebSocket
// upgrade route on the supplied workspace-scoped router group (wsScoped, i.e.
// .../projects/:projectId/repos/:repoId/workspaces/:wsId), mirroring
// terminal.Register. Every AgentChat is anchored to exactly one workspace, so
// nesting here gives Create/List/Get/Switch/Rename/Handoff/Delete the :wsId
// path param the handlers scope by, and gives the whole surface
// scopeWorkspaceToPath's wsId-ownership enforcement (router.go) for free. The
// WS route lands in the SAME group so its :wsId is available to
// agentChatDef's Filter (container.go) — the resulting route is
// .../workspaces/:wsId/chats/ws.
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
// The chat FOLDER routes mount on the same group and for the same reason: a chat
// folder is workspace-scoped, it shares one sibling space with the chats above
// it, and .../chats/folders is where a client already looks for everything about
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
// settingsRG is the top-level /v0 group. Provider PRIORITY + enable/disable is a
// GLOBAL user setting (per user/machine, not per workspace — the CLIs are
// machine-level), so its write route mounts there at /settings/chat/providers,
// outside the entity hierarchy — mirroring /settings/terminal/profiles. It is the
// counterpart of the workspace-scoped enriched GET .../chats/providers above, and
// is mounted exactly once (the home group re-mounts the GET but never this write).
func Register(
	wsScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	chats agenthandlers.ChatUsecase,
	turns agenthandlers.TurnUsecase,
	runners agenthandlers.RunnerUsecase,
	answers agenthandlers.AnswerUsecase,
	providers agenthandlers.ProviderUsecase,
	folders agenthandlers.ChatTreeUsecase,
	broadcastFolder func(folderID, workspaceID, kind string),
	wsHandle gin.HandlerFunc,
) {
	h := agenthandlers.New(chats, turns, runners, answers, providers, folders, broadcastFolder)

	wsScoped.POST("/chats", h.Create)
	wsScoped.GET("/chats", h.List)
	wsScoped.GET("/chats/:id", h.Get)
	wsScoped.GET("/chats/:id/messages", h.Messages)
	wsScoped.POST("/chats/:id/prompts", h.SubmitPrompt)
	wsScoped.GET("/chats/:id/activity", h.Activity)
	wsScoped.GET("/chats/:id/activity/:toolId/payload", h.ToolPayload)
	wsScoped.GET("/chats/:id/choices", h.Choices)
	wsScoped.POST("/chats/:id/choices/:choiceId/answer", h.AnswerChoice)
	wsScoped.GET("/chats/:id/telemetry", h.Telemetry)
	wsScoped.GET("/chats/:id/slash-catalog", h.SlashCatalog)
	wsScoped.POST("/chats/:id/switch", h.Switch)
	wsScoped.POST("/chats/:id/resume", h.Resume)
	wsScoped.POST("/chats/:id/compact", h.Compact)
	wsScoped.POST("/chats/:id/stop", h.Stop)
	wsScoped.POST("/chats/:id/rename", h.Rename)
	wsScoped.PATCH("/chats/:id/selection", h.SetSelection)
	wsScoped.GET("/chats/:id/handoff", h.Handoff)
	wsScoped.PATCH("/chats/:id/placement", h.PlaceChat)
	wsScoped.DELETE("/chats/:id", h.Delete)
	wsScoped.GET("/chats/folders", h.ListFolders)
	wsScoped.POST("/chats/folders", h.CreateFolder)
	wsScoped.PATCH("/chats/folders/:folderId", h.PatchFolder)
	wsScoped.DELETE("/chats/folders/:folderId", h.DeleteFolder)
	wsScoped.POST("/chats/runners/:segid/mcp", h.MCP)
	wsScoped.POST("/chats/hooks", h.Hooks)
	wsScoped.POST("/chats/hooks/await", h.AwaitHookAnswer)
	wsScoped.POST("/chats/hooks/abandon", h.AbandonHookAnswer)
	wsScoped.GET("/chats/providers", h.Providers)
	wsScoped.GET("/chats/ws", wsHandle)

	settingsRG.PUT("/settings/chat/providers", h.UpdateProviderPreferences)
}
