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

// GetOutline handles GET /v0/chats/:chatId/review/outline, returning the
// hunk geometry of the branch diff. It is what lets the client reserve the
// right scroll space for every file before fetching a single patch, so it
// answers the shape of a million-line diff without any of its content.
func (h *Handlers) GetOutline(
	ctx *gin.Context,
) {
	files, err := h.reviewUsecase.GetOutline(ctx.Request.Context(), h.workspaceID(ctx), scopeCommit(ctx))
	if err != nil {
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
		return
	}
	if files == nil {
		files = []gitdomain.FileOutline{}
	}
	writeOutline(ctx, outlineResponse{Files: files})
}

// writeOutline streams the outline envelope rather than rendering it with
// libs.WriteQueryOK, for two reasons that both come from its size.
//
// The outline is the one v0 payload that is large by design: a million-line
// branch produces ~38,600 hunks, and at four JSON numbers under four names
// apiece that is 2.3 MB. gin's JSON render marshals into a byte slice before
// writing, so serving it that way would make the daemon hold the whole response
// — the exact O(diff) allocation this endpoint family exists to abolish, merely
// moved from the diff to its description. Encoding straight into the writer
// holds one buffer instead.
//
// The same repetition is what makes it compress: the four field names recur
// once per hunk, which is what DEFLATE is best at, and the wire cost drops by
// roughly an order of magnitude. Field names are NOT shortened to buy the same
// bytes: they are the published shape of gitdomain.FileOutline, and a second,
// abbreviated vocabulary for the same four numbers would have to be learned and
// maintained by every consumer, where the encoding is transparent to all of
// them.
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
	defer func() { _ = zw.Close() }()
	encodeOutline(ctx, zw, data)
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
		_ = ctx.Error(err)
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
