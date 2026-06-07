package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestListSuccess(
	t *testing.T,
) {
	f := &fakeLifecycle{
		listChats: []domain.Chat{
			{
				ID:        "c1",
				WsID:      "w1",
				Title:     "alpha",
				Status:    domain.ChatStatusIdle,
				CreatedAt: time.Unix(1, 0).UTC(),
			},
		},
	}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/w1/chats", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", f.gotListWs)
	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID    string `json:"id"`
			WsID  string `json:"wsId"`
			Title string `json:"title"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Success)
	require.Len(t, body.Data, 1)
	assert.Equal(t, "c1", body.Data[0].ID)
	assert.Equal(t, "alpha", body.Data[0].Title)
}

func TestListEmptyYieldsArray(
	t *testing.T,
) {
	rec := do(newRouter(&fakeLifecycle{}), http.MethodGet, "/v0/workspaces/w1/chats", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

func TestListNotFound(
	t *testing.T,
) {
	f := &fakeLifecycle{listErr: apperr.ErrNotFound}
	rec := do(newRouter(f), http.MethodGet, "/v0/workspaces/nope/chats", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
