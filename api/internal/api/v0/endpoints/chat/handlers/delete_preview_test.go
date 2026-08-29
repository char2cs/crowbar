package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// TestDeletePreview_ReturnsChatAndFileCounts proves the handler forwards :id
// verbatim to the tree usecase and renders its two counts under the wire
// shape the delete confirm dialog reads (chatCount, fileCount).
func TestDeletePreview_ReturnsChatAndFileCounts(t *testing.T) {
	tree := &fakeChatTree{previewChats: 2, previewFiles: 6}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodGet, "/chats/folder-1/delete-preview", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "folder-1"}}

	newFolderHandlers(tree, &frames).DeletePreview(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "folder-1", tree.gotPreviewID)
	var env struct {
		Data struct {
			ChatCount int `json:"chatCount"`
			FileCount int `json:"fileCount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, 2, env.Data.ChatCount)
	assert.Equal(t, 6, env.Data.FileCount)
}

// TestDeletePreview_UnknownIDIs404 proves a not-found from the usecase maps to
// 404 rather than a bare 200 with zeroed counts.
func TestDeletePreview_UnknownIDIs404(t *testing.T) {
	tree := &fakeChatTree{err: apperr.ErrNotFound}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodGet, "/chats/nowhere/delete-preview", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "nowhere"}}

	newFolderHandlers(tree, &frames).DeletePreview(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
