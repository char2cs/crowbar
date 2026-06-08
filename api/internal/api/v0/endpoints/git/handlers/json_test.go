package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// doRaw sends raw (possibly invalid) JSON to trigger ShouldBindJSON failures.
func doRaw(
	r *gin.Engine,
	method, path, raw string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// TestGitWriteHandlers_BadJSON exercises the ShouldBindJSON error branches.
func TestGitWriteHandlers_BadJSON(
	t *testing.T,
) {
	r := newRouter()
	bad := `"not-an-object"`

	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/stage", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/stage-hunk", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/unstage", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/unstage-hunk", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/discard", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/commit", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/branches", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPatch, ws+"/branches", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/switch", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/stash-apply", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/stash-pop", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/reset", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/merge", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/rebase", bad).Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, ws+"/resolve-hunk", bad).Code)
}
