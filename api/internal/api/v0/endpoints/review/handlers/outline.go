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

// outlineResponse is the data payload of GET /review/outline: every changed
// file's hunk geometry and nothing else.
type outlineResponse struct {
	Files []gitdomain.FileOutline `json:"files"`
}

// GetOutline handles GET /v0/chats/:chatId/review/outline, returning the branch diff's hunk
// geometry so the client can reserve scroll space for every file before fetching any patch.
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

// writeOutline encodes straight into the writer (gzip when accepted) instead of libs.WriteQueryOK:
// the outline is large by design (megabytes for a huge branch), and gin's render would hold it all.
// Its repeated field names compress well, so they stay unabbreviated.
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

// encodeOutline records an encode failure on the context rather than replacing
// the response: by the time one can happen the status line is committed and the
// only honest thing left is to stop writing.
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

// acceptsGzip reports whether the client offered gzip. An explicit refusal
// ("gzip;q=0") is honoured; any other weight is still an offer.
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
