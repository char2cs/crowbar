// Package worktree mounts the worktree surface a chat addresses: the seven lifecycle verbs on the
// worktree it holds, the branch rename, and the batch branch import. Requests name the chat,
// never a workspace id.
package worktree

import (
	"github.com/gin-gonic/gin"

	worktreehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree/handlers"
)

// Register mounts the chat-keyed worktree routes at .../chats/:id on the repo-scoped group, beside
// chat's promote/rename, from here because their bodies are worktreehandlers methods. Batch import
// is its own route because it resolves the open-PR graph across the set, which N creates cannot.
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
	// Without a resolver the verbs cannot answer which worktree a chat holds, so
	// they are not mounted at all rather than left to answer a fiction.
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
	// A PATCH on the branch, not a second meaning for POST /chats/:id/rename (see ChatRenameBranch).
	rg.PATCH("/chats/:id/branch", h.ChatRenameBranch)
}
