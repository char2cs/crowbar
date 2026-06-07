package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

func TestCreateFileSuccess(
	t *testing.T,
) {
	f := &fakeFiles{}
	rec := do(
		newRouter(f),
		http.MethodPost,
		"/v0/workspaces/w1/files",
		`{"path":"new.txt","type":"file"}`,
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "new.txt", f.gotCreateFile.path)
	assert.Equal(t, "", f.gotCreateDir.path)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "new.txt", body.Data.ID)
}

func TestCreateFolderSuccess(
	t *testing.T,
) {
	f := &fakeFiles{}
	rec := do(
		newRouter(f),
		http.MethodPost,
		"/v0/workspaces/w1/files",
		`{"path":"dir","type":"folder"}`,
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "dir", f.gotCreateDir.path)
	assert.Equal(t, "", f.gotCreateFile.path)
}

func TestCreateDirectoryTypeSuccess(
	t *testing.T,
) {
	f := &fakeFiles{}
	rec := do(
		newRouter(f),
		http.MethodPost,
		"/v0/workspaces/w1/files",
		`{"path":"dir","type":"directory"}`,
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "dir", f.gotCreateDir.path)
}

func TestCreateBadJSON(
	t *testing.T,
) {
	rec := do(newRouter(&fakeFiles{}), http.MethodPost, "/v0/workspaces/w1/files", `{bad`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateMissingPath(
	t *testing.T,
) {
	rec := do(newRouter(&fakeFiles{}), http.MethodPost, "/v0/workspaces/w1/files", `{"type":"file"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateUnknownType(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeFiles{}),
		http.MethodPost,
		"/v0/workspaces/w1/files",
		`{"path":"x","type":"symlink"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateMissingType(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeFiles{}),
		http.MethodPost,
		"/v0/workspaces/w1/files",
		`{"path":"x"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateError(
	t *testing.T,
) {
	f := &fakeFiles{createFileErr: apperr.ErrNotFound}
	rec := do(
		newRouter(f),
		http.MethodPost,
		"/v0/workspaces/w1/files",
		`{"path":"x","type":"file"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRenameSuccess(
	t *testing.T,
) {
	f := &fakeFiles{}
	rec := do(
		newRouter(f),
		http.MethodPatch,
		"/v0/workspaces/w1/files",
		`{"path":"a.txt","newPath":"b.txt"}`,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "a.txt", f.gotRename.oldPath)
	assert.Equal(t, "b.txt", f.gotRename.newPath)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "b.txt", body.Data.ID)
}

func TestRenameBadJSON(
	t *testing.T,
) {
	rec := do(newRouter(&fakeFiles{}), http.MethodPatch, "/v0/workspaces/w1/files", `{bad`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameMissingPath(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeFiles{}),
		http.MethodPatch,
		"/v0/workspaces/w1/files",
		`{"newPath":"b.txt"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameMissingNewPath(
	t *testing.T,
) {
	rec := do(
		newRouter(&fakeFiles{}),
		http.MethodPatch,
		"/v0/workspaces/w1/files",
		`{"path":"a.txt"}`,
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameError(
	t *testing.T,
) {
	f := &fakeFiles{renameErr: apperr.ErrNotFound}
	rec := do(
		newRouter(f),
		http.MethodPatch,
		"/v0/workspaces/w1/files",
		`{"path":"a.txt","newPath":"b.txt"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteSuccess(
	t *testing.T,
) {
	f := &fakeFiles{}
	rec := do(
		newRouter(f),
		http.MethodDelete,
		"/v0/workspaces/w1/files",
		`{"path":"a.txt"}`,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "a.txt", f.gotDelete.path)
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "a.txt", body.Data.ID)
}

func TestDeleteBadJSON(
	t *testing.T,
) {
	rec := do(newRouter(&fakeFiles{}), http.MethodDelete, "/v0/workspaces/w1/files", `{bad`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteMissingPath(
	t *testing.T,
) {
	rec := do(newRouter(&fakeFiles{}), http.MethodDelete, "/v0/workspaces/w1/files", `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteError(
	t *testing.T,
) {
	f := &fakeFiles{deleteErr: apperr.ErrNotFound}
	rec := do(
		newRouter(f),
		http.MethodDelete,
		"/v0/workspaces/w1/files",
		`{"path":"a.txt"}`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
