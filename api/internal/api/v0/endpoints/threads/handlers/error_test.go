package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestThreadHandlers_NotFound(t *testing.T) {
	store := &fakeStore{err: problem.ErrNotFound}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	assert.Equal(t, http.StatusNotFound, do(r, http.MethodGet, base, nil).Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodGet, base+"/t1", nil).Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodPost, base+"/t1/replies",
		map[string]any{"body": "x"}).Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodPatch, base+"/t1",
		map[string]any{"isResolved": true}).Code)
	assert.Empty(t, bc.pushed)
}

func TestThreadHandlers_InternalError(t *testing.T) {
	store := &fakeStore{err: assertErr{}}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, base, nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, base+"/t1", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPatch, base+"/t1",
		map[string]any{"isResolved": false}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, base,
		map[string]any{"filePath": "x.go", "body": "c"}).Code)
}

func TestThreadHandlers_AFullQueueIsUnavailable(t *testing.T) {
	store := &fakeStore{err: fmt.Errorf("review thread: reply t1: %w", fmt.Errorf("dispatch: %w", asynxModels.ErrQueueFull))}
	r := newRouter(store, &fakeBroadcaster{})

	rec := do(r, http.MethodPost, base+"/t1/replies", map[string]any{"body": "x"})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"success":false,"code":"unavailable","error":"dispatch: asynx: queue full"}`, rec.Body.String())
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }

func TestThreadHandlers_BadJSON(t *testing.T) {
	store := &fakeStore{thread: sampleThread()}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, base, "{bad").Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPost, base+"/t1/replies", "{bad").Code)
	assert.Equal(t, http.StatusBadRequest, doRaw(r, http.MethodPatch, base+"/t1", "{bad").Code)
}

func TestThreadHandlers_NilBroadcaster(t *testing.T) {
	store := &fakeStore{thread: sampleThread()}
	r := newRouter(store, nil)

	assert.Equal(t, http.StatusCreated, do(r, http.MethodPost, base,
		map[string]any{"filePath": "main.go", "line": 1, "side": "right", "body": "c"}).Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodPost, base+"/t1/replies",
		map[string]any{"body": "r"}).Code)
}
