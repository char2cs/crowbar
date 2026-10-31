package documents

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Documents is the file usecase surface the handlers need.
type Documents interface {
	ReadDocument(
		ctx context.Context,
		target domain.DocumentRef,
		transport domain.Transport,
	) (domain.Document, error)
	WriteDocument(
		ctx context.Context,
		target domain.DocumentRef,
		content string,
		encoding string,
		transport domain.Transport,
		now time.Time,
	) (domain.DocumentRef, error)
}

type handlers struct {
	docs Documents
}

// Read handles GET /v0/documents/content?workspace=<id>&path=<rel> and
// GET /v0/documents/content?path=<abs>.
func (h *handlers) Read(
	ctx *gin.Context,
) {
	target := targetOf(ctx.Query("workspace"), ctx.Query("path"))
	if target.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}
	doc, err := h.docs.ReadDocument(ctx.Request.Context(), target, domain.TransportOf(ctx.Request.Context()))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, doc)
}

// Write handles PUT /v0/documents/content and answers the ref the write
// resolved to.
func (h *handlers) Write(
	ctx *gin.Context,
) {
	var body struct {
		WorkspaceID string `json:"workspaceId"`
		Path        string `json:"path"`
		Content     string `json:"content"`
		Encoding    string `json:"encoding"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	target := targetOf(body.WorkspaceID, body.Path)
	if target.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}
	ref, err := h.docs.WriteDocument(ctx.Request.Context(), target, body.Content, body.Encoding, domain.TransportOf(ctx.Request.Context()), time.Now())
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, ref)
}

// targetOf is a workspace ref when the request names a workspace, else an
// absolute path for the usecase to resolve.
func targetOf(
	workspaceID string,
	path string,
) domain.DocumentRef {
	if workspaceID == "" {
		return domain.DocumentRef{Path: path}
	}
	return domain.DocumentRef{Authority: domain.DocumentAuthorityWorkspace, WorkspaceID: workspaceID, Path: path}
}
