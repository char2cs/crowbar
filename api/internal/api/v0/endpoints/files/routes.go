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
// is no chat resolving to it. It reuses the SAME filesWS broadcaster passed to
// Register here (router.go), which is why filesDef (container.go) still
// carries a wsId filter alongside chatId: home injects one, this mount never
// does.
//
// filesWS is the WebSocket handler for the live file-change stream. Unlike
// git's status route it is NOT dual-served: it is its own /files/ws leaf,
// because the file tree's REST read (/files/tree) answers a directory level
// while the stream carries change events — two different shapes, so there is
// no single route a GET and an upgrade could both mean.
func Register(
	chatScoped *gin.RouterGroup,
	files filehandlers.Files,
	filesWS gin.HandlerFunc,
) {
	h := filehandlers.New(files)
	mount(chatScoped, "/files", h, filesWS)
}

// mount registers the 8-route files surface under prefix on rg.
func mount(
	rg *gin.RouterGroup,
	prefix string,
	h *filehandlers.Handlers,
	filesWS gin.HandlerFunc,
) {
	rg.GET(prefix+"/content", h.ReadContent)
	rg.GET(prefix+"/tree", h.Tree)
	rg.PUT(prefix+"/content", h.SaveContent)
	rg.POST(prefix, h.Create)
	rg.POST(prefix+"/copy", h.Copy)
	rg.PATCH(prefix, h.Rename)
	rg.DELETE(prefix, h.Delete)
	rg.GET(prefix+"/ws", filesWS)
}
