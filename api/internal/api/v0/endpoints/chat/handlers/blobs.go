package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// BlobReader reads the payloads a chat's rows name by ref.
type BlobReader interface {
	ReadBlob(ctx context.Context, chatID, ref string) ([]byte, error)
}

// Blobs serves GET /v0/chats/:chatId/blobs/:ref.
type Blobs struct{ blobs BlobReader }

// NewBlobs builds the blob handler over blobs.
func NewBlobs(blobs BlobReader) *Blobs { return &Blobs{blobs: blobs} }

// Handle answers the payload's bytes as they were stored.
func (h *Blobs) Handle(c *gin.Context) {
	data, err := h.blobs.ReadBlob(c.Request.Context(), c.Param("chatId"), c.Param("ref"))
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteUntrustedFile(c, "text/plain; charset=utf-8", data)
}
