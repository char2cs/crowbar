package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

type errSearchEngine struct{ err error }

func (e errSearchEngine) Search(
	_ context.Context,
	_ string,
	_ enginesearch.SearchRequest,
) (enginesearch.SearchResponse, error) {
	return enginesearch.SearchResponse{}, e.err
}

func (e errSearchEngine) Replace(
	_ context.Context,
	_ string,
	_ enginesearch.ReplaceRequest,
	_ bool,
) error {
	return e.err
}

func TestSearchHandlers_BadPattern(
	t *testing.T,
) {
	r2 := newRouterWith(errSearchEngine{err: enginesearch.ErrBadPattern})

	rec := do(r2, http.MethodPost, "/v0/chats/chat1/search",
		map[string]any{"query": "("})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = do(r2, http.MethodPost, "/v0/chats/chat1/search/replace",
		map[string]any{"query": "("})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSearchHandlers_ReplaceErrors(
	t *testing.T,
) {
	r := newRouterWith(errSearchEngine{err: enginesearch.ErrLocked})
	rec := do(r, http.MethodPost, "/v0/chats/chat1/search/replace",
		map[string]any{"query": "fmt"})
	assert.Equal(t, http.StatusForbidden, rec.Code)

	r2 := newRouterWith(errSearchEngine{err: enginesearch.ErrPathOutsideWorkspace})
	rec = do(r2, http.MethodPost, "/v0/chats/chat1/search/replace",
		map[string]any{"query": "fmt"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
