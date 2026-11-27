package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

type BlobReader interface {
	ReadBlob(ctx context.Context, chatID, ref string) ([]byte, error)
}

type Blobs struct{ blobs BlobReader }

func NewBlobs(blobs BlobReader) *Blobs { return &Blobs{blobs: blobs} }

func (h *Blobs) Handle(c *gin.Context) {
	data, err := h.blobs.ReadBlob(c.Request.Context(), c.Param("chatId"), c.Param("ref"))
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteUntrustedFile(c, "text/plain; charset=utf-8", data)
}
