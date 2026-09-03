// Package files mounts the v0 file REST routes and the co-located files
// WebSocket upgrade route.
package files

import (
	"github.com/gin-gonic/gin"

	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
)

// Register mounts the files REST and WebSocket surface on BOTH scoping groups
// it is currently addressable through.
//
// files is spec §4.2's shared bucket: the worktree holds one tree, and every
// chat holding that worktree reads and writes it. chatScoped is where that
// lives from now on — /v0/chats/:chatId/files/... , the flat prefix §7.1 closes
// on — and the frontend talks to it exclusively for worktree-backed workspaces.
//
// wsScoped is the OLD /projects/:projectId/repos/:repoId/workspaces/:wsId/files/...
// surface, mounted unchanged. It is not a fallback and nothing chooses between
// the two: it is simply a route that has not been retired yet, and retiring it
// is spec §8 step 6's job. Deleting THIS call is the whole of files' share of
// that step.
//
// A third mount exists and is deliberately untouched here: home.Register serves
// /projects/:projectId/home/files/... from its own handlers for the
// project-level row that has no repo and no worktree, which is why the frontend
// keeps addressing THAT one by project rather than by chat — there is no chat
// resolving to it. §4.1 deletes the home group wholesale, also in step 6.
//
// One route table serves both groups here, so the two can never drift into
// different surfaces: mount is called twice with different prefixes, and a
// route added to it appears on both by construction. The handlers themselves
// take the worktree from whichever mount the request arrived on — see
// handlers.workspaceID.
//
// filesWS is the WebSocket handler for the live file-change stream. Unlike
// git's status route it is NOT dual-served: it is its own /files/ws leaf on
// each prefix, because the file tree's REST read (/files/tree) answers a
// directory level while the stream carries change events — two different
// shapes, so there is no single route a GET and an upgrade could both mean.
func Register(
	wsScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	files filehandlers.Files,
	filesWS gin.HandlerFunc,
) {
	h := filehandlers.New(files)
	mount(chatScoped, "/files", h, filesWS)
	mount(wsScoped, "/workspaces/:wsId/files", h, filesWS)
}

// mount registers the 8-route files surface under prefix on rg. It is the
// single definition of that surface; Register calls it once per live scoping
// group.
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
