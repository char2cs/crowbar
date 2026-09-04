// Package worktree mounts the worktree surface a CHAT addresses: the seven
// lifecycle verbs on the thing actually being held, the branch rename, and the
// batch branch import.
//
// It is what remains of the old `workspaces` endpoint group after spec §8 step
// 6 (docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md). That group's
// thirteen :wsId routes are deleted — every one had a chat-keyed replacement
// live and in use — and the package is named for what it now serves rather than
// for the id nobody may name any more (law 1).
package worktree

import (
	"github.com/gin-gonic/gin"

	worktreehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree/handlers"
)

// Register mounts the chat-keyed worktree routes on the supplied repo-scoped
// group. reader, hierarchy and repos back the verbs' reads and worktree
// operations; lastErrors and working carry an async op's outcome on the entity;
// remote lets the batch import refuse an absent branch synchronously; worktrees
// resolves a chat to the workspace behind its worktree (spec §3).
//
// The verbs mount on the repo-scoped group at :id — the param a sibling
// .../chats/:id/promote already binds at that position — so lock/sync/merge sit
// beside promote/rename/placement rather than in a second, differently-shaped
// chat surface.
//
// They are registered from HERE rather than from chat.Register because the
// bodies they run are these handlers': every one is a worktreehandlers.Handlers
// method, so re-declaring the reader, the hierarchy, the work signal, the error
// sink and runAsync in the chat package to serve them would be a second copy of
// this file's entire dependency set.
//
// .../chats/import-batch is the BATCH import, and it is a route of its own
// beside POST .../chats rather than a loop over it. POST .../chats with an
// `import` body adopts ONE named branch; this one takes a set and resolves the
// repo's open-PR head→base graph across it, creating the ancestors a branch is
// parented under and falling back to a placeholder row for a branch another
// worktree already holds. Driving it as N single creates would silently drop all
// three of those.
func Register(
	rg *gin.RouterGroup,
	reader worktreehandlers.Reader,
	hierarchy worktreehandlers.Hierarchy,
	repos worktreehandlers.Repos,
	lastErrors worktreehandlers.LastErrorSetter,
	working worktreehandlers.WorkSignal,
	remote worktreehandlers.RemoteRefs,
	worktrees worktreehandlers.Worktrees,
) {
	h := worktreehandlers.New(reader, hierarchy, repos, lastErrors, working).
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
	// The chat-keyed branch rename. It is a PATCH on the branch and NOT a second
	// meaning for POST /chats/:id/rename — see ChatRenameBranch for why that fold
	// was declined.
	rg.PATCH("/chats/:id/branch", h.ChatRenameBranch)
}
