package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

func TestSearch_EmptyQuery_400(t *testing.T) {
	r := newRouter(t, enginesearch.New(), stubReader{path: t.TempDir()})

	w := doPost(t, r, "/v0/workspaces/ws1/search", `{"query":""}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}

func TestSearch_MalformedBody_400(t *testing.T) {
	r := newRouter(t, enginesearch.New(), stubReader{path: t.TempDir()})

	w := doPost(t, r, "/v0/workspaces/ws1/search", `{`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearch_BadRegex_400(t *testing.T) {
	r := newRouter(t, enginesearch.New(), stubReader{path: t.TempDir()})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search",
		`{"query":"[invalid","regex":true}`,
	)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}

func TestSearch_HappyPath_200(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "src/main.go", "hello world\n")
	r := newRouter(t, enginesearch.New(), stubReader{path: root})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search",
		`{"query":"hello","caseSensitive":true}`,
	)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)
	assert.True(t, env.Success)

	var data dto.SearchResponseDTO
	require.NoError(t, json.Unmarshal(env.Data, &data))
	require.Len(t, data.Results, 1)
	assert.Equal(t, "src/main.go", data.Results[0].FilePath)
	assert.Equal(t, 1, data.Results[0].LineNumber)
}

func TestSearch_NoMatches_NonNilEmptyResults(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "a.txt", "nothing here\n")
	r := newRouter(t, enginesearch.New(), stubReader{path: root})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search",
		`{"query":"zzz","caseSensitive":true}`,
	)

	require.Equal(t, http.StatusOK, w.Code)
	env := decodeEnvelope(t, w)
	assert.True(t, env.Success)

	type rawData struct {
		Results json.RawMessage `json:"results"`
	}
	var raw rawData
	require.NoError(t, json.Unmarshal(env.Data, &raw))
	assert.Equal(t, "[]", string(raw.Results))
}
