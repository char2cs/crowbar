package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// UploadAttachment handles POST .../workspaces/:wsId/chats/:id/attachments.
//
// Two ingestion shapes, the same precedent icons.ReadUpload already
// establishes (api/internal/api/v0/endpoints/icons/icons.go), adapted for
// attachments' own larger size cap and its own required "id" field:
//   - multipart/form-data: a "file" field (clipboard paste, browser file
//     picker — neither ever has a host path) plus an "id" form field.
//   - application/json: {"path": "...", "id": "..."} — a revived desktop
//     drag-and-drop, which yields a host path rather than bytes; the daemon
//     reads it itself. Same residual trust assumption icons.go's own
//     path-read variant documents: daemon and webview share a host, so this
//     is a user-chosen path from a native drop, not attacker-controlled.
//
// id is required on both shapes and never minted here — the frontend already
// generates a nanoid per attachment, and accepting rather than generating one
// is what lets that id double as an Excalidraw fence-tag id.
func (h *Handlers) UploadAttachment(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	data, id, originalName, ok := readAttachmentUpload(ctx)
	if !ok {
		return
	}
	stored, err := h.turns.UploadAttachment(ctx.Request.Context(), chat.ID, agentusecase.UploadAttachmentInput{
		ID: id, OriginalName: originalName, Data: data,
	})
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryWithStatus(ctx, http.StatusCreated, dto.ChatAttachmentDTO{
		Ref: stored.Ref, FileName: stored.FileName, Size: stored.Size, ContentType: stored.ContentType,
	})
}

func readAttachmentUpload(ctx *gin.Context) (data []byte, id, originalName string, ok bool) {
	if strings.HasPrefix(ctx.ContentType(), "application/json") {
		return readAttachmentFromPath(ctx)
	}
	return readAttachmentFromMultipart(ctx)
}

func readAttachmentFromMultipart(ctx *gin.Context) ([]byte, string, string, bool) {
	id := ctx.Request.FormValue("id")
	if id == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "id required")
		return nil, "", "", false
	}
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "file field required")
		return nil, "", "", false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, repoattachments.MaxBytes+1))
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, "read error")
		return nil, "", "", false
	}
	if int64(len(data)) > repoattachments.MaxBytes {
		libs.WriteErr(ctx, http.StatusRequestEntityTooLarge, "attachment exceeds the size limit")
		return nil, "", "", false
	}
	return data, id, resolveOriginalName(header.Filename, data), true
}

// resolveOriginalName returns filename verbatim when the client supplied one,
// or synthesizes a fallback from the sniffed content type otherwise —
// a defensive backstop for a blank filename (some non-browser multipart
// encoders omit it; net/http's own multipart parser cannot even represent a
// FILE part with an explicitly empty filename, so this path is exercised only
// by callers other than Go's stdlib client). The frontend (Task 20) still
// sends a real name whenever it can; this is the backend's own last resort.
func resolveOriginalName(filename string, data []byte) string {
	if filename != "" {
		return filename
	}
	return repoattachments.SyntheticName(repoattachments.ContentType(data), func() string {
		return time.Now().UTC().Format("20060102T150405")
	})
}

// readAttachmentFromPath reads the attachment from an absolute host path
// supplied as JSON — see the residual trust assumption in UploadAttachment's
// own doc comment.
func readAttachmentFromPath(ctx *gin.Context) ([]byte, string, string, bool) {
	var body struct {
		Path string `json:"path"`
		ID   string `json:"id"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil || body.Path == "" || body.ID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path and id required")
		return nil, "", "", false
	}
	info, err := os.Stat(body.Path)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "could not read attachment file")
		return nil, "", "", false
	}
	if info.IsDir() {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is a directory, not a file")
		return nil, "", "", false
	}
	if info.Size() > repoattachments.MaxBytes {
		libs.WriteErr(ctx, http.StatusRequestEntityTooLarge, "attachment exceeds the size limit")
		return nil, "", "", false
	}
	//nolint:gosec // G304: path is an absolute host path from a native file dialog/drop, the same residual trust model as icons.go's readFromPath (daemon and webview share a host).
	f, err := os.Open(body.Path)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "could not read attachment file")
		return nil, "", "", false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, repoattachments.MaxBytes+1))
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, "read error")
		return nil, "", "", false
	}
	if int64(len(data)) > repoattachments.MaxBytes {
		libs.WriteErr(ctx, http.StatusRequestEntityTooLarge, "attachment exceeds the size limit")
		return nil, "", "", false
	}
	return data, body.ID, filepath.Base(body.Path), true
}

// Attachment handles GET .../workspaces/:wsId/chats/:id/attachments/:file,
// serving one durable attachment's raw bytes with a sniffed Content-Type — the
// asset-serving endpoint the frontend's MarkdownAssetContext resolver calls to
// turn a stored ![]()/[]() ref into fetchable pixels (design spec,
// "Rendering in the composer & transcript"). Unlike
// .../activity/:toolId/payload (its closest shape precedent), this endpoint
// sniffs the real Content-Type rather than hardcoding text/plain, since an
// attachment is exactly as likely to be an image or a PDF as text.
func (h *Handlers) Attachment(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	data, contentType, err := h.turns.ReadAttachment(ctx.Request.Context(), chat.ID, ctx.Param("file"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	ctx.Header("Cache-Control", "no-cache")
	ctx.Data(http.StatusOK, contentType, data)
}
