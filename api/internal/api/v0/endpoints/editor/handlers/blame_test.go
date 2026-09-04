package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestBlame_200(t *testing.T) {
	git := &fakeGit{entries: []gitdomain.BlameEntry{
		{
			LineNumber:    1,
			CommitHash:    "abc",
			Author:        "Ada",
			Email:         "ada@x.io",
			Date:          time.Unix(1000, 0).UTC(),
			CommitMessage: "init",
		},
	}}
	r := newRouter(&fakeLSP{}, git)

	rec := do(t, r, http.MethodGet, "/v0/chats/ws1/blame?path=main.go", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	var entries []dto.BlameEntryDTO
	require.NoError(t, json.Unmarshal(env.Data, &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "abc", entries[0].CommitHash)
	assert.Equal(t, "1970-01-01T00:16:40Z", entries[0].Date)
}

func TestBlame_EmptyArrayNotNull(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{})

	rec := do(t, r, http.MethodGet, "/v0/chats/ws1/blame?path=main.go", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	assert.Equal(t, "[]", string(env.Data))
}

func TestBlame_MissingPath_400(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{})

	rec := do(t, r, http.MethodGet, "/v0/chats/ws1/blame", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	env := decode(t, rec)
	assert.False(t, env.Success)
	assert.Equal(t, "path is required", env.Error)
}

// TestBlame_NoResolvedWorkspace_500 is the wiring-bug guard: a route mounted
// outside chatScoped's resolveChatWorktree middleware finds no workspace on
// reqscope and reports a 500 rather than silently acting on the empty string
// (Handlers.workspace).
func TestBlame_NoResolvedWorkspace_500(t *testing.T) {
	r := newRouterNoWorkspace(&fakeLSP{}, &fakeGit{})

	rec := do(t, r, http.MethodGet, "/v0/chats/chat1/blame?path=main.go", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.False(t, decode(t, rec).Success)
}

func TestBlame_EngineError_500(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{err: errBoom})

	rec := do(t, r, http.MethodGet, "/v0/chats/ws1/blame?path=main.go", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.False(t, decode(t, rec).Success)
}

func TestBlame_NilEngine_503(t *testing.T) {
	r := newRouter(&fakeLSP{}, nil)

	rec := do(t, r, http.MethodGet, "/v0/chats/ws1/blame?path=main.go", nil)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	env := decode(t, rec)
	assert.False(t, env.Success)
	assert.Equal(t, "git engine not available", env.Error)
}
