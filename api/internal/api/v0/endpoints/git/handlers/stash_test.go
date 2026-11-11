package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// stashRecorder is stubGit that remembers what StashPush and StashDrop got.
type stashRecorder struct {
	stubGit
	message, dropped string
}

func (s *stashRecorder) StashPush(_ context.Context, _, message string, _ time.Time) error {
	s.message = message
	return nil
}

func (s *stashRecorder) StashDrop(_ context.Context, _, stashID string, _ time.Time) error {
	s.dropped = stashID
	return nil
}

// The stash body is optional, so an empty one keeps the defaults; one that is
// there but does not decode is the client's mistake, not an empty body.
func TestStashPush_AMalformedBodyIs400AndAnEmptyOneKeepsTheDefaults(t *testing.T) {
	git := &stashRecorder{}
	r := newRouterWith(git)
	raw := func(method, path, body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, raw(http.MethodPost, ws+"/stash", ""))
	assert.Empty(t, git.message)
	assert.Equal(t, http.StatusOK, raw(http.MethodPost, ws+"/stash", `{"message":"wip"}`))
	assert.Equal(t, "wip", git.message)
	assert.Equal(t, http.StatusBadRequest, raw(http.MethodPost, ws+"/stash", `{"message":`))

	assert.Equal(t, http.StatusNoContent, raw(http.MethodDelete, ws+"/stash?index=2", ""))
	assert.Equal(t, "2", git.dropped)
	assert.Equal(t, http.StatusBadRequest, raw(http.MethodDelete, ws+"/stash?index=2", `{"index":"x"}`))
}
