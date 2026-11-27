package worktree

import (
	"github.com/gin-gonic/gin"

	worktreehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree/handlers"
)

func Register(
	rg *gin.RouterGroup,
	reader worktreehandlers.Reader,
	hierarchy worktreehandlers.Hierarchy,
	repos worktreehandlers.Repos,
	ops worktreehandlers.Operations,
	chats worktreehandlers.Chats,
	remote worktreehandlers.RemoteRefs,
	worktrees worktreehandlers.Worktrees,
) {
	h := worktreehandlers.New(reader, hierarchy, repos, ops, chats).
		WithRemoteRefs(remote)
	rg.POST("/chats/import-batch", h.Import)
	if worktrees == nil {
		return
	}
	h.WithWorktrees(worktrees)
	rg.POST("/chats/:id/lock", h.ChatLock)
	rg.POST("/chats/:id/sync", h.ChatSync)
	rg.POST("/chats/:id/merge-into-parent", h.ChatMergeIntoParent)
	rg.POST("/chats/:id/reparent", h.ChatReparent)
	rg.POST("/chats/:id/rebase-onto-parent", h.ChatRebaseOntoParent)
	rg.POST("/chats/:id/retry-provision", h.ChatRetryProvision)
	rg.POST("/chats/:id/detach-holder", h.ChatDetachHolder)
	rg.PATCH("/chats/:id/branch", h.ChatRenameBranch)
}
