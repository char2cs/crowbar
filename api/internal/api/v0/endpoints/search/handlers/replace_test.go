package handlers_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

func TestReplace_MalformedBody_400(t *testing.T) {
	r := newRouter(t, enginesearch.New(), stubReader{path: t.TempDir()})

	w := doPost(t, r, "/v0/workspaces/ws1/search/replace", `{`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReplace_EmptyQuery_400(t *testing.T) {
	r := newRouter(t, enginesearch.New(), stubReader{path: t.TempDir()})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search/replace",
		`{"query":"","replacement":"x","scope":"all"}`,
	)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReplace_BadPattern_400(t *testing.T) {
	r := newRouter(t, enginesearch.New(), stubReader{path: t.TempDir()})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search/replace",
		`{"query":"[invalid","replacement":"x","scope":"all","regex":true}`,
	)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}

func TestReplace_HappyPath_200(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "file.go", "hello world\n")
	r := newRouter(t, enginesearch.New(), stubReader{path: root})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search/replace",
		`{"query":"hello","replacement":"goodbye","scope":"all","caseSensitive":true}`,
	)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, decodeEnvelope(t, w).Success)

	data, err := os.ReadFile(filepath.Join(root, "file.go"))
	require.NoError(t, err)
	assert.Equal(t, "goodbye world\n", string(data))
}

func TestReplace_LockedWorkspace_409(t *testing.T) {
	root := t.TempDir()
	seedFile(t, root, "file.go", "hello\n")
	r := newRouter(
		t,
		enginesearch.New(),
		stubReader{path: root, locked: true},
	)

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search/replace",
		`{"query":"hello","replacement":"bye","scope":"all","caseSensitive":true}`,
	)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}

func TestReplace_PathTraversal_400(t *testing.T) {
	r := newRouter(t, enginesearch.New(), stubReader{path: t.TempDir()})

	w := doPost(
		t,
		r,
		"/v0/workspaces/ws1/search/replace",
		`{"query":"hello","replacement":"bye","scope":"file:../../etc/passwd","caseSensitive":true}`,
	)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, decodeEnvelope(t, w).Success)
}
