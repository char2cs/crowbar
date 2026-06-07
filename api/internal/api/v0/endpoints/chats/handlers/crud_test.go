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

func TestCreateSuccess(
	t *testing.T,
) {
	f := &fakeLifecycle{created: domain.Chat{ID: "new-id"}}
	rec := do(
		newRouter(f),
		http.MethodPost,
		"/v0/workspaces/w1/chats",
		`{"title":"hello"}`,
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "w1", f.gotCreateWs)
	assert.Equal(t, "hello", f.gotCreateTit)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "new-id", body.Data.ID)
}

func TestCreateEmptyBodyTolerated(
	t *testing.T,
) {
	f := &fakeLifecycle{created: domain.Chat{ID: "x"}}
	rec := do(newRouter(f), http.MethodPost, "/v0/workspaces/w1/chats", "")

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "", f.gotCreateTit)
}

func TestCreateBadJSON(
	t *testing.T,
) {
	rec := do(newRouter(&fakeLifecycle{}), http.MethodPost, "/v0/workspaces/w1/chats", `{bad`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateNotFound(
	t *testing.T,
) {
	f := &fakeLifecycle{createErr: apperr.ErrNotFound}
	rec := do(newRouter(f), http.MethodPost, "/v0/workspaces/nope/chats", `{"title":"t"}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestForkSuccess(
	t *testing.T,
) {
	f := &fakeLifecycle{forked: domain.Chat{ID: "child"}}
	rec := do(newRouter(f), http.MethodPost, "/v0/chats/parent/fork", "")

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "parent", f.gotForkID)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "child", body.Data.ID)
}

func TestForkNotFound(
	t *testing.T,
) {
	f := &fakeLifecycle{forkErr: apperr.ErrNotFound}
	rec := do(newRouter(f), http.MethodPost, "/v0/chats/nope/fork", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRenameSuccess(
	t *testing.T,
) {
	f := &fakeLifecycle{renamed: domain.Chat{ID: "c1"}}
	rec := do(newRouter(f), http.MethodPatch, "/v0/chats/c1", `{"title":"renamed"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "c1", f.gotRenameID)
	assert.Equal(t, "renamed", f.gotRenameTit)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "c1", body.Data.ID)
}

func TestRenameBadJSON(
	t *testing.T,
) {
	rec := do(newRouter(&fakeLifecycle{}), http.MethodPatch, "/v0/chats/c1", `{bad`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameMissingTitle(
	t *testing.T,
) {
	rec := do(newRouter(&fakeLifecycle{}), http.MethodPatch, "/v0/chats/c1", `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameEmptyTitle(
	t *testing.T,
) {
	rec := do(newRouter(&fakeLifecycle{}), http.MethodPatch, "/v0/chats/c1", `{"title":""}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameNotFound(
	t *testing.T,
) {
	f := &fakeLifecycle{renameErr: apperr.ErrNotFound}
	rec := do(newRouter(f), http.MethodPatch, "/v0/chats/nope", `{"title":"t"}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteSuccess(
	t *testing.T,
) {
	f := &fakeLifecycle{}
	rec := do(newRouter(f), http.MethodDelete, "/v0/chats/c1", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "c1", f.gotDeleteID)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "c1", body.Data.ID)
}

func TestDeleteNotFound(
	t *testing.T,
) {
	f := &fakeLifecycle{deleteErr: apperr.ErrNotFound}
	rec := do(newRouter(f), http.MethodDelete, "/v0/chats/nope", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
