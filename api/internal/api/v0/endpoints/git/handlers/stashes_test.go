package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestStashesSuccess(
	t *testing.T,
) {
	git := &fakeGit{
		stashes: []gitdomain.Stash{
			{ID: "stash@{0}", Message: "wip", Date: time.Unix(0, 0).UTC(), FilesChanged: 2},
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/stashes")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []struct {
			ID           string `json:"id"`
			Message      string `json:"message"`
			Date         string `json:"date"`
			FilesChanged int    `json:"filesChanged"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "stash@{0}", body.Data[0].ID)
	assert.Equal(t, "wip", body.Data[0].Message)
	assert.Equal(t, "1970-01-01T00:00:00Z", body.Data[0].Date)
	assert.Equal(t, 2, body.Data[0].FilesChanged)
}

func TestStashesEmptyNonNil(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/stashes")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

func TestStashesError(
	t *testing.T,
) {
	git := &fakeGit{stashesErr: errors.New("boom")}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/stashes")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
