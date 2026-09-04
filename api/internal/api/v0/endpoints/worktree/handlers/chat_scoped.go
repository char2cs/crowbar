package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// The worktree lifecycle verbs (spec
// docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md §4.3): Lock,
// Sync, MergeIntoParent, Reparent, RebaseOntoParent, RetryProvision and
// DetachHolder are verbs on the thing actually being HELD, and past law 1 the
// only thing a request may name is a chat.
//
// They were once each one body reached two ways — a :wsId keying beside this
// one — behind a workspaceTarget indirection that let the pair share every
// guard. Spec §8 step 6 deleted the :wsId twins, so the indirection has one
// implementation and nothing to keep in step: each verb is now a single method
// that resolves its chat and acts.

// chatWorkspace answers which workspace a request means: the URL names a CHAT,
// and the workspace is the one behind the worktree that chat reads and writes
// through — itself if it owns one, else its nearest ancestor that does (spec
// §3). It writes the error response and returns ok=false when the caller must
// stop.
//
// Every resolve failure answers the same 404, which is exactly what
// resolveChatWorktree (the flat /chats/:chatId group's own middleware) already
// does and for the same reason: from the caller's side a chat whose worktree
// cannot be resolved — a bubble hanging off nothing
// (worktree.ErrNoWorktreeInAncestry) included — is indistinguishable from a
// chat that does not exist. The resolve is also the existence check the verbs
// make before acting: it loads the workspace to return it.
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
// the worktree a chat holds.
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
// all live below this handler in hierarchy.RenameBranch's own guardRenameBranch.
// The raw PATCH /v0/chats/:chatId/git/branches is NOT that: it takes the old
// name from the caller, enforces none of those, and leaves
// domain.Workspace.Branch stale — which is why this exists rather than pointing
// a client at it.
//
// The body is bound BEFORE the target is resolved, matching lock's own order:
// a malformed request is refused for what is wrong with it rather than for a
// chat lookup it never got to.
//
// It answers with the CHAT id. Past law 1 a workspace has no id a client may
// hold, and handing one back would be the read-only workspaceId §6 explicitly
// rejects.
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
