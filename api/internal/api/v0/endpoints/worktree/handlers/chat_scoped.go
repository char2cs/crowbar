package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The worktree lifecycle verbs act on the worktree a chat holds; a request only ever names the
// chat, and each verb resolves it to its workspace before acting.

// chatWorkspace resolves the URL's chat to the workspace behind its worktree (its own, else its
// nearest ancestor's), writing a 404 and returning ok=false on any failure: like
// resolveChatWorktree, an unresolvable chat is indistinguishable from a missing one.
func (h *Handlers) chatWorkspace(
	c *gin.Context,
) (domain.Workspace, bool) {
	ws, err := h.worktrees.Resolve(c.Request.Context(), c.Param("id"))
	if err != nil {
		libs.WriteErr(c, http.StatusNotFound, "chat not found")
		return domain.Workspace{}, false
	}
	return ws, true
}

// startOn starts run as an operation on ws and answers 202 with it, or the refusal.
func (h *Handlers) startOn(
	c *gin.Context,
	ws domain.Workspace,
	kind domain.OperationKind,
	label string,
	run func(ctx context.Context) error,
) {
	op, err := h.ops.Start(c.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(c),
		Kind:      kind,
		Subject:   domain.EntityRef{Kind: domain.EntityWorkspace, ID: ws.ID},
		ProjectID: ws.ProjectID,
		Label:     strings.TrimSpace(label),
	}, func(ctx context.Context, _ operation.Progress) error {
		return run(ctx)
	})
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteOperation(c, op)
}

// relLabel is "<head> <rel> <branch of id>", or head alone when id's branch
// cannot be read: a label is never left dangling.
func (h *Handlers) relLabel(
	ctx context.Context,
	head string,
	rel string,
	id string,
) string {
	all, err := h.reader.List(ctx)
	if err != nil {
		return head
	}
	for _, ws := range all {
		if ws.ID == id && ws.Branch != "" {
			return head + " " + rel + " " + ws.Branch
		}
	}
	return head
}

// renameBranchRequest is the PATCH .../chats/:id/branch body; the branch renamed FROM is the
// daemon's to know, never the caller's to assert.
type renameBranchRequest struct {
	Branch string `json:"branch"`
}

// ChatRenameBranch handles PATCH .../chats/:id/branch, renaming the branch of the chat's worktree.
// It is separate from the title-only POST .../rename because title and branch are independent and
// differently guarded; hierarchy.RenameBranch owns the guards. It answers with the chat id.
func (h *Handlers) ChatRenameBranch(
	c *gin.Context,
) {
	var body renameBranchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	if err := h.applyRename(c, ws.ID, &body.Branch); err != nil {
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("id"))
}
