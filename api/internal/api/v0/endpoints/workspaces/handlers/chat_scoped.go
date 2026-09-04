package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// The chat-keyed half of the worktree lifecycle verbs (spec
// docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md §4.3): Lock,
// Sync, MergeIntoParent, Reparent, RebaseOntoParent, RetryProvision and
// DetachHolder are verbs on the thing actually being HELD, and past law 1 the
// only thing a request may name is a chat.
//
// Each verb is therefore one body reached two ways, never two implementations:
// the handler asks a workspaceTarget which workspace this request means, and
// everything after that — the guards, the 202/204 split, the async bracket, the
// broadcast — is the code the :wsId route has always run. That is the whole
// point of the shape. A chat-keyed Sync and a workspace-keyed Sync cannot drift
// because there is nothing to drift: they differ in the target function and in
// nothing else.
//
// The :wsId twins stay mounted until spec §8 step 6b retires them.

// workspaceTarget answers which workspace a request means, writing the error
// response and returning ok=false when the caller must stop. It is the ONE
// difference between the two keyings of every verb below.
type workspaceTarget func(c *gin.Context) (string, bool)

// namedWorkspace is the :wsId keying: the URL names the workspace outright and
// the read is the existence check the verbs have always made before acting.
func (h *Handlers) namedWorkspace(
	c *gin.Context,
) (string, bool) {
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return "", false
	}
	return id, true
}

// chatWorkspace is the :id keying: the URL names a CHAT, and the workspace is
// the one behind the worktree that chat reads and writes through — itself if it
// owns one, else its nearest ancestor that does (spec §3).
//
// Every resolve failure answers the same 404, which is exactly what
// resolveChatWorktree (the flat /chats/:chatId group's own middleware) already
// does and for the same reason: from the caller's side a chat whose worktree
// cannot be resolved — a bubble hanging off nothing
// (worktree.ErrNoWorktreeInAncestry) included — is indistinguishable from a
// chat that does not exist. The resolve subsumes namedWorkspace's existence
// check: it loads the workspace to return it.
func (h *Handlers) chatWorkspace(
	c *gin.Context,
) (string, bool) {
	ws, err := h.worktrees.Resolve(c.Request.Context(), c.Param("id"))
	if err != nil {
		libs.WriteErr(c, http.StatusNotFound, "chat not found")
		return "", false
	}
	return ws.ID, true
}

// renameBranchRequest is the PATCH .../chats/:id/branch body: the whole of what
// this verb takes, since the branch being renamed FROM is the daemon's own
// answer and never the caller's to assert.
type renameBranchRequest struct {
	Branch string `json:"branch"`
}

// ChatRenameBranch handles PATCH .../chats/:id/branch: renames the branch of
// the worktree a chat holds, with every guard the :wsId PATCH's own rename half
// enforces.
//
// It is a SEPARATE verb from POST .../chats/:id/rename, which stays title-only.
// Spec §7.5 proposed folding the two — "renaming a worktree-owning chat renames
// the branch, renaming a bubble renames the title" — and that is the one
// decision here taken against the spec text, deliberately: a chat's title and
// its branch are independently meaningful (the sidebar draws both), the fold
// makes a title edit silently rewrite a git ref for some rows and not others,
// and the two carry different guards and different failure modes. One verb that
// does two different irreversible things depending on what the row turns out to
// be is not one verb.
//
// The guards are not reimplemented here, and that is the point of routing
// through applyRename: locked-branch refusal, the unprovisioned-placeholder
// refusal, the repo-wide name-collision check and the adopted-checkout refusal
// all live below this handler in hierarchy.RenameBranch's own guardRenameBranch,
// so the chat-keyed door is exactly as strict as the workspace-keyed one. The
// raw PATCH /v0/chats/:chatId/git/branches is NOT that: it takes the old name
// from the caller, enforces none of those, and leaves domain.Workspace.Branch
// stale — which is why this exists rather than pointing a client at it.
//
// The body is bound BEFORE the target is resolved, matching lock's own order:
// a malformed request is refused for what is wrong with it rather than for a
// chat lookup it never got to.
//
// It answers with the CHAT id, not the workspace's. Past law 1 a workspace has
// no id a client may hold, and handing one back on a chat-keyed route would be
// the read-only workspaceId §6 explicitly rejects.
func (h *Handlers) ChatRenameBranch(
	c *gin.Context,
) {
	var body renameBranchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	wsID, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	if err := h.applyRename(c, wsID, &body.Branch); err != nil {
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("id"))
}
