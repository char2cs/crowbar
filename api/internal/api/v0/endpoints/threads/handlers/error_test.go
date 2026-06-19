package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// TestThreadHandlers_NotFound asserts a not-found store error maps to 404 on the
// read and mutation paths that resolve an existing thread.
func TestThreadHandlers_NotFound(t *testing.T) {
	store := &fakeStore{err: apperr.ErrNotFound}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	assert.Equal(t, http.StatusNotFound, do(r, http.MethodGet, base, nil).Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodGet, base+"/t1", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodPost, base+"/t1/replies",
		map[string]any{"body": "x"}).Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodPatch, base+"/t1",
		map[string]any{"isResolved": true}).Code)
	// No mutation broadcasts when the store rejects the command.
	assert.Empty(t, bc.pushed)
}

// TestThreadHandlers_InternalError asserts a non-not-found store error maps to
// 500, never masking a real failure as a 404.
func TestThreadHandlers_InternalError(t *testing.T) {
	store := &fakeStore{err: assertErr{}}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, base, nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, base+"/t1", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPatch, base+"/t1",
		map[string]any{"isResolved": false}).Code)
	// OpenThread fail-fast maps store errors to 400 (invalid anchor input).
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, base,
		map[string]any{"filePath": "x.go", "body": "c"}).Code)
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }

// TestThreadHandlers_BadJSON asserts a malformed body is rejected with 400 on
// every body-binding route before the store is touched.
func TestThreadHandlers_BadJSON(t *testing.T) {
	store := &fakeStore{thread: sampleThread()}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, base, "{bad").Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, base+"/t1/replies", "{bad").Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPatch, base+"/t1", "{bad").Code)
}

// TestThreadHandlers_NilBroadcaster asserts the handlers are safe when no
// broadcaster is wired: mutations still succeed and simply skip the push.
func TestThreadHandlers_NilBroadcaster(t *testing.T) {
	store := &fakeStore{thread: sampleThread()}
	r := newRouter(store, nil)

	assert.Equal(t, http.StatusCreated, do(r, http.MethodPost, base,
		map[string]any{"filePath": "main.go", "line": 1, "side": "right", "body": "c"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, base+"/t1/replies",
		map[string]any{"body": "r"}).Code)
}
