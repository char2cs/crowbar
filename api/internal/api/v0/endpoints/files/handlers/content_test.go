package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestReadContentSuccess(
	t *testing.T,
) {
	f := &fakeFiles{content: domain.FileContent{Content: "hello", Encoding: ""}}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/w1/files/content?path=a.txt", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", f.gotRead.ws)
	assert.Equal(t, "a.txt", f.gotRead.path)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "hello", body.Data.Content)
	assert.Equal(t, "", body.Data.Encoding)
}

func TestReadContentBinaryEncoding(
	t *testing.T,
) {
	f := &fakeFiles{content: domain.FileContent{Content: "AAAA", Encoding: "base64"}}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/w1/files/content?path=img.png", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Encoding string `json:"encoding"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "base64", body.Data.Encoding)
}

func TestReadContentMissingPath(
	t *testing.T,
) {
	rec := do(newRouter(&fakeFiles{}), http.MethodGet, "/v0/workspaces/w1/files/content", "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReadContentNotFound(
	t *testing.T,
) {
	f := &fakeFiles{contentErr: apperr.ErrNotFound}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/w1/files/content?path=x", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSaveContentSuccess(
	t *testing.T,
) {
	f := &fakeFiles{}
	rec := do(
		newRouter(f),
		http.MethodPut,
		"/v0/workspaces/w1/files/content",
		`{"path":"a.txt","content":"body"}`,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", f.gotWrite.ws)
	assert.Equal(t, "a.txt", f.gotWrite.path)
	assert.Equal(t, "body", f.gotWrite.content)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "a.txt", body.Data.ID)
}

func TestSaveContentBadJSON(
	t *testing.T,
) {
	rec := do(newRouter(&fakeFiles{}), http.MethodPut, "/v0/workspaces/w1/files/content", `{bad`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSaveContentMissingPath(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeFiles{}),
		http.MethodPut,
		"/v0/workspaces/w1/files/content",
		`{"content":"body"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSaveContentError(
	t *testing.T,
) {
	f := &fakeFiles{writeErr: apperr.ErrNotFound}
	rec := do(
		newRouter(f),
		http.MethodPut,
		"/v0/workspaces/w1/files/content",
		`{"path":"a.txt","content":"body"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
