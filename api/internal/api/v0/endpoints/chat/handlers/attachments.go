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

// UploadAttachment handles POST .../chats/:id/attachments as multipart ("file" plus "id") or JSON
// {"path","id"} for a desktop drop that yields a host path. The client-minted id is required, never
// generated here, so it can double as an Excalidraw fence-tag id.
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
		libs.Fail(ctx, err)
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
	defer libs.CloseLogged(ctx.Request.Context(), "chat attachments: close the upload", file)
	data, ok := readAttachmentBytes(ctx, file)
	if !ok {
		return nil, "", "", false
	}
	return data, id, resolveOriginalName(header.Filename, data), true
}

// readAttachmentBytes enforces MaxBytes on the bytes actually read (cap+1 detects overflow without
// unbounded buffering), so a stream larger than an earlier size check promised is still refused.
func readAttachmentBytes(ctx *gin.Context, r io.Reader) ([]byte, bool) {
	data, err := io.ReadAll(io.LimitReader(r, repoattachments.MaxBytes+1))
	if err != nil {
		libs.WriteErr(ctx, http.StatusInternalServerError, "read error")
		return nil, false
	}
	if int64(len(data)) > repoattachments.MaxBytes {
		libs.WriteErr(ctx, http.StatusRequestEntityTooLarge, "attachment exceeds the size limit")
		return nil, false
	}
	return data, true
}

// resolveOriginalName synthesizes a name from the sniffed content type when the client sent a blank
// filename, which some non-browser multipart encoders do.
func resolveOriginalName(filename string, data []byte) string {
	if filename != "" {
		return filename
	}
	return repoattachments.SyntheticName(repoattachments.ContentType(data), func() string {
		return time.Now().UTC().Format("20060102T150405")
	})
}

// readAttachmentFromPath reads an absolute host path supplied as JSON. Daemon and webview share a host,
// so the path is a user's native drop, not attacker-controlled.
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
	defer libs.CloseLogged(ctx.Request.Context(), "chat attachments: close the file", f)
	data, ok := readAttachmentBytes(ctx, f)
	if !ok {
		return nil, "", "", false
	}
	return data, body.ID, filepath.Base(body.Path), true
}

// Attachment serves one durable attachment's raw bytes, so a stored markdown
// ref resolves to something fetchable; the Content-Type is sniffed because an
// attachment is as likely an image or a PDF as text, and an HTML or SVG one must not run on open.
func (h *Handlers) Attachment(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	data, contentType, err := h.turns.ReadAttachment(ctx.Request.Context(), chat.ID, ctx.Param("file"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-cache")
	libs.WriteUntrustedFile(ctx, contentType, data)
}
