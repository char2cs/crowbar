// Package workspaces mounts the v0 workspace REST routes, including the
// worktree hierarchy operations and the dual-served list that upgrades to the
// live workspaces WebSocket stream on demand (02 §2.2).
package workspaces

import (
	"github.com/gin-gonic/gin"

	workspacehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces/handlers"
)

// Register mounts the workspace list, detail, create, delete, and hierarchy
// routes on the supplied router group. The list AND detail routes are
// dual-served: a plain GET returns REST while a WebSocket upgrade is routed to
// wsHandle for the live stream — the list-scope subscriber receives the repo's
// workspaces, the :wsId-scope subscriber receives exactly that workspace (W7-2).
// reader and hierarchy back the reads and worktree operations; repos resolves a
// repository for worktree-backed create; placer and chats resolve and move a
// workspace-owning row's chat placement in the unified sidebar tree; worktrees
// resolves a chat to the workspace behind its worktree for the chat-keyed verbs
// below; dispatch wraps the dual-served routes so the upgrade is honoured
// without a second path.
//
// The .../chats/:id/… block is spec §4.3's move of the seven worktree LIFECYCLE
// verbs onto the thing actually being held. It mounts on the same repo-scoped
// group chat's own verbs use (:id, not :chatId — the param a sibling
// .../chats/:id/promote already binds at that position), so lock/sync/merge sit
// beside promote/rename/placement rather than in a second, differently-shaped
// chat surface.
//
// They are registered from HERE rather than from chat.Register because the
// bodies they run are these handlers': every one of the seven is one
// workspacehandlers.Handlers method reached through a different
// workspaceTarget, so re-declaring the reader, the hierarchy, the work signal,
// the error sink and runAsync in the chat package to serve them would be a
// second copy of this file's entire dependency set. Spec §8 step 6b retires the
// :wsId twins above; what is left here is exactly this block.
func Register(
	rg *gin.RouterGroup,
	reader workspacehandlers.Reader,
	hierarchy workspacehandlers.Hierarchy,
	repos workspacehandlers.Repos,
	lastErrors workspacehandlers.LastErrorSetter,
	working workspacehandlers.WorkSignal,
	remote workspacehandlers.RemoteRefs,
	placer workspacehandlers.Placer,
	chats workspacehandlers.ChatResolver,
	worktrees workspacehandlers.Worktrees,
	broadcastFolder func(folderID string, workspaceID string, kind string),
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := workspacehandlers.New(reader, hierarchy, repos, lastErrors, working).
		WithRemoteRefs(remote).
		WithPlacer(placer, chats, broadcastFolder).
		WithWorktrees(worktrees)
	rg.GET("/workspaces", dispatch(h.List, wsHandle))
	rg.GET("/workspaces/:wsId", dispatch(h.Detail, wsHandle))
	rg.POST("/workspaces", h.Create)
	rg.POST("/workspaces/import", h.Import)
	rg.PATCH("/workspaces/:wsId", h.Patch)
	rg.DELETE("/workspaces/:wsId", h.Delete)
	rg.POST("/workspaces/:wsId/sync", h.Sync)
	rg.POST("/workspaces/:wsId/lock", h.Lock)
	rg.POST("/workspaces/:wsId/merge-into-parent", h.MergeIntoParent)
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
	rg.POST("/workspaces/:wsId/rebase-onto-parent", h.RebaseOntoParent)
	rg.POST("/workspaces/:wsId/retry-provision", h.RetryProvision)
	rg.POST("/workspaces/:wsId/detach-holder", h.DetachHolder)
	if worktrees == nil {
		return
	}
	rg.POST("/chats/:id/lock", h.ChatLock)
	rg.POST("/chats/:id/sync", h.ChatSync)
	rg.POST("/chats/:id/merge-into-parent", h.ChatMergeIntoParent)
	rg.POST("/chats/:id/reparent", h.ChatReparent)
	rg.POST("/chats/:id/rebase-onto-parent", h.ChatRebaseOntoParent)
	rg.POST("/chats/:id/retry-provision", h.ChatRetryProvision)
	rg.POST("/chats/:id/detach-holder", h.ChatDetachHolder)
}
