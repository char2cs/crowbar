package handlers

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type outlineResponse struct {
	Files []gitdomain.FileOutline `json:"files"`
}

func (h *Handlers) GetOutline(
	ctx *gin.Context,
) {
	files, err := h.reviewUsecase.GetOutline(ctx.Request.Context(), h.workspaceID(ctx), scopeCommit(ctx))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	if files == nil {
		files = []gitdomain.FileOutline{}
	}
	writeOutline(ctx, outlineResponse{Files: files})
}

func writeOutline(
	ctx *gin.Context,
	data outlineResponse,
) {
	ctx.Header("Content-Type", "application/json; charset=utf-8")
	ctx.Header("Vary", "Accept-Encoding")
	ctx.Status(http.StatusOK)
	if !acceptsGzip(ctx.Request.Header.Get("Accept-Encoding")) {
		encodeOutline(ctx, ctx.Writer, data)
		return
	}
	ctx.Header("Content-Encoding", "gzip")
	zw := gzip.NewWriter(ctx.Writer)
	encodeOutline(ctx, zw, data)
	if err := zw.Close(); err != nil {
		libs.NoteError(ctx, err)
	}
}

func encodeOutline(
	ctx *gin.Context,
	w io.Writer,
	data outlineResponse,
) {
	err := json.NewEncoder(w).Encode(libs.Envelope{Success: true, Data: data})
	if err != nil {
		libs.NoteError(ctx, err)
	}
}

func acceptsGzip(
	header string,
) bool {
	for _, coding := range strings.Split(header, ",") {
		if offersGzip(strings.TrimSpace(coding)) {
			return true
		}
	}
	return false
}

func offersGzip(
	coding string,
) bool {
	name, params, _ := strings.Cut(coding, ";")
	if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
		return false
	}
	return !isRefused(params)
}

func isRefused(
	params string,
) bool {
	_, weight, ok := strings.Cut(strings.ReplaceAll(params, " ", ""), "q=")
	if !ok {
		return false
	}
	q, err := strconv.ParseFloat(weight, 64)
	return err == nil && q == 0
}
