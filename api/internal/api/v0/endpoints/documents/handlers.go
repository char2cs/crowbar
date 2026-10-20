package documents

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The error codes a client tells the document refusals apart by.
const (
	// HostDocumentRequiresLocalTransport refuses a host document asked for off
	// the local socket.
	HostDocumentRequiresLocalTransport = "host_document_requires_local_transport"
	// DaemonStateDocument refuses a path inside the daemon's state directory.
	DaemonStateDocument = "daemon_state_document"
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
	doc, err := h.docs.ReadDocument(ctx.Request.Context(), target, transportOf(ctx.Request))
	if err != nil {
		documentError(ctx, err)
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
	ref, err := h.docs.WriteDocument(ctx.Request.Context(), target, body.Content, body.Encoding, transportOf(ctx.Request), time.Now())
	if err != nil {
		documentError(ctx, err)
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

// transportOf is local only for a request accepted on a unix socket; a request
// that names no listener fails closed as remote.
func transportOf(
	r *http.Request,
) domain.Transport {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if ok && addr.Network() == "unix" {
		return domain.TransportLocal
	}
	return domain.TransportRemote
}

func documentError(
	ctx *gin.Context,
	err error,
) {
	status, msg := libs.StatusAndMessage(err)
	switch {
	case errors.Is(err, apperr.ErrHostDocumentNeedsLocalTransport):
		libs.WriteErrCode(ctx, status, HostDocumentRequiresLocalTransport, msg)
	case errors.Is(err, apperr.ErrDaemonStateDocument):
		libs.WriteErrCode(ctx, status, DaemonStateDocument, msg)
	default:
		libs.WriteErr(ctx, status, msg)
	}
}
