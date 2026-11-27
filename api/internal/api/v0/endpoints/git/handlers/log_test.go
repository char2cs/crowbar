package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

type logGit struct {
	stubGit
	commits []gitdomain.Commit
	err     error
}

func (g logGit) Log(_ context.Context, _ string, _, _ int) ([]gitdomain.Commit, error) {
	return g.commits, g.err
}

func TestLog_EmptyRepo(
	t *testing.T,
) {
	r := newRouterWith(logGit{commits: []gitdomain.Commit{}})

	rec := do(r, http.MethodGet, ws+"/log", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []gitdomain.Commit `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Data)
}

func TestLog_WithCommits(
	t *testing.T,
) {
	commits := []gitdomain.Commit{
		{Hash: "abc123def", ShortHash: "abc123d", Message: "feat: add thing", Author: "dev", Date: time.Unix(1, 0).UTC()},
		{Hash: "def456abc", ShortHash: "def456a", Message: "fix: bug", Author: "dev", Date: time.Unix(2, 0).UTC()},
	}
	r := newRouterWith(logGit{commits: commits})

	rec := do(r, http.MethodGet, ws+"/log?limit=10&skip=0", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []gitdomain.Commit `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
	assert.Equal(t, "abc123def", body.Data[0].Hash)
}

func TestLog_InvalidLimit(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodGet, ws+"/log?limit=notanumber", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLog_InvalidSkip(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodGet, ws+"/log?skip=notanumber", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestLog_UnknownRef(
	t *testing.T,
) {
	r := newRouterWith(logGit{err: enginegit.ErrBranchNotFound})

	rec := do(r, http.MethodGet, ws+"/log", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
