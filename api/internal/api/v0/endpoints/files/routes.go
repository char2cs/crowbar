// Package files mounts the v0 file REST routes and the co-located files
// WebSocket upgrade route.
package files

import (
	"github.com/gin-gonic/gin"

	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
)

// Register mounts the files REST and WebSocket surface on the flat
// chat-scoped group (spec §7.1): /v0/chats/:chatId/files/... .
//
// files is spec §4.2's shared bucket: the worktree holds one tree, and every
// chat holding that worktree reads and writes it, resolved from the request
// context by chatScoped's own resolveChatWorktree middleware (see
// handlers.Handlers.workspaceID).
//
// The old /projects/:projectId/repos/:repoId/workspaces/:wsId/files/... mount
// is gone (spec §8 step 6): every caller had already moved to the mount kept
// here.
//
// A second mount remains and is deliberately untouched here: home.Register
// serves /projects/:projectId/home/files/... from its own handlers for the
// project-level row that has no repo and no worktree, which is why the
// frontend keeps addressing THAT one by project rather than by chat — there
// is no chat resolving to it.
func Register(
	chatScoped *gin.RouterGroup,
	files filehandlers.Files,
) {
	h := filehandlers.New(files)
	mount(chatScoped, "/files", h)
}

// mount registers the 6-route files surface under prefix on rg. A file's
// content is read and written through /v0/documents/content.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *filehandlers.Handlers,
) {
	rg.GET(prefix+"/tree", h.Tree)
	rg.POST(prefix, h.Create)
	rg.POST(prefix+"/copy", h.Copy)
	rg.PATCH(prefix, h.Rename)
	rg.DELETE(prefix, h.Delete)
}
