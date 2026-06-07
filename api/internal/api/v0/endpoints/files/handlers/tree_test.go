package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestTreeSuccess(
	t *testing.T,
) {
	f := &fakeFiles{
		tree: []domain.FileNode{
			{Name: "src", Path: "src", Type: domain.FileNodeTypeDirectory},
			{Name: "go.mod", Path: "go.mod", Type: domain.FileNodeTypeFile},
		},
	}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/w1/files/tree?path=sub", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", f.gotTree.ws)
	assert.Equal(t, "sub", f.gotTree.path)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	require.Len(t, body.Data, 2)
	assert.Equal(t, "src", body.Data[0].Name)
}

func TestTreeEmptyYieldsArray(
	t *testing.T,
) {
	f := &fakeFiles{tree: nil}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/w1/files/tree", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "", f.gotTree.path)
	assert.JSONEq(t, `{"success":true,"data":[]}`, rec.Body.String())
}

func TestTreeError(
	t *testing.T,
) {
	f := &fakeFiles{treeErr: apperr.ErrNotFound}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/nope/files/tree", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTreeInternalError(
	t *testing.T,
) {
	f := &fakeFiles{treeErr: errors.New("boom")}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/w1/files/tree", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
