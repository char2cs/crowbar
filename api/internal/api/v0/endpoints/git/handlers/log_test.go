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

func TestLogSuccessDefaults(
	t *testing.T,
) {
	git := &fakeGit{
		commits: []gitdomain.Commit{
			{Hash: "abc", ShortHash: "abc", Message: "init", Author: "me", Date: time.Unix(0, 0).UTC()},
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/log")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 50, git.gotLimit)
	assert.Equal(t, 0, git.gotSkip)
	var body struct {
		Data []struct {
			Hash string `json:"hash"`
			Date string `json:"date"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "abc", body.Data[0].Hash)
	assert.Equal(t, "1970-01-01T00:00:00Z", body.Data[0].Date)
}

func TestLogParsesLimitAndSkip(
	t *testing.T,
) {
	git := &fakeGit{}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/log?limit=10&skip=5")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 10, git.gotLimit)
	assert.Equal(t, 5, git.gotSkip)
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

func TestLogBadLimit(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/log?limit=abc")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLogNegativeLimit(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/log?limit=-1")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLogBadSkip(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/log?skip=x")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLogError(
	t *testing.T,
) {
	git := &fakeGit{logErr: errors.New("boom")}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/log")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
