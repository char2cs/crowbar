package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
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
	r2 := newRouterWith(errSearchEngine{err: enginesearch.ErrBadPattern}, stubReader{})

	rec := do(r2, http.MethodPost, "/v0/workspaces/ws1/search",
		map[string]any{"query": "("})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = do(r2, http.MethodPost, "/v0/workspaces/ws1/search/replace",
		map[string]any{"query": "("})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSearchHandlers_ReplaceErrors(
	t *testing.T,
) {
	r := newRouterWith(errSearchEngine{err: enginesearch.ErrLocked}, stubReader{})
	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/search/replace",
		map[string]any{"query": "fmt"})
	assert.Equal(t, http.StatusForbidden, rec.Code)

	r2 := newRouterWith(errSearchEngine{err: enginesearch.ErrPathOutsideWorkspace}, stubReader{})
	rec = do(r2, http.MethodPost, "/v0/workspaces/ws1/search/replace",
		map[string]any{"query": "fmt"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// errReader always fails the workspace lookup, so Search/Replace's "workspace
// not found" 404 path can be exercised independently of the search engine.
type errReader struct{ err error }

func (e errReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, e.err
}

// TestSearchHandlers_UnknownWorkspace_Returns404 proves Search 404s when the
// workspace lookup fails, BEFORE the search engine is ever consulted — a
// stale or malformed :wsId must not reach the engine with an empty
// WorktreePath.
func TestSearchHandlers_UnknownWorkspace_Returns404(
	t *testing.T,
) {
	r := newRouterWith(stubEngine{}, errReader{err: errors.New("no such workspace")})

	rec := do(r, http.MethodPost, "/v0/workspaces/ghost/search",
		map[string]any{"query": "fmt"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestReplaceHandlers_UnknownWorkspace_Returns404 is Search's proof mirrored
// onto Replace: the workspace lookup failing must 404 before Replace is ever
// called.
func TestReplaceHandlers_UnknownWorkspace_Returns404(
	t *testing.T,
) {
	r := newRouterWith(stubEngine{}, errReader{err: errors.New("no such workspace")})

	rec := do(r, http.MethodPost, "/v0/workspaces/ghost/search/replace",
		map[string]any{"query": "fmt", "replacement": "log"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestSearchHandlers_EngineUnavailable_Returns503 proves Search refuses with
// 503, not a panic or a 500, when the daemon was wired up with no search
// engine at all — h.searchEng == nil is a real deployment state (the engine
// failed to construct), not a test-only impossibility.
func TestSearchHandlers_EngineUnavailable_Returns503(
	t *testing.T,
) {
	r := newRouterWith(nil, stubReader{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/search",
		map[string]any{"query": "fmt"})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestReplaceHandlers_EngineUnavailable_Returns503 is the Replace-side
// counterpart of TestSearchHandlers_EngineUnavailable_Returns503.
func TestReplaceHandlers_EngineUnavailable_Returns503(
	t *testing.T,
) {
	r := newRouterWith(nil, stubReader{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/search/replace",
		map[string]any{"query": "fmt", "replacement": "log"})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestHandleSearchError_UnrecognisedErrorIsA500WithAFixedMessage proves
// handleSearchError's default branch: an error that is not ErrBadPattern
// answers with the FIXED "search failed" message, never the raw err.Error()
// text, so an engine-internal error string is never leaked to the client.
func TestHandleSearchError_UnrecognisedErrorIsA500WithAFixedMessage(
	t *testing.T,
) {
	r := newRouterWith(errSearchEngine{err: errors.New("ripgrep: exit status 2")}, stubReader{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/search",
		map[string]any{"query": "fmt"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "search failed")
	assert.NotContains(t, rec.Body.String(), "ripgrep",
		"the raw engine error must not leak to the client")
}

// TestHandleReplaceError_UnrecognisedErrorIsA500WithItsOwnMessage proves
// handleReplaceError's default branch, which — unlike handleSearchError's —
// surfaces the error's own text rather than a fixed message.
func TestHandleReplaceError_UnrecognisedErrorIsA500WithItsOwnMessage(
	t *testing.T,
) {
	r := newRouterWith(errSearchEngine{err: errors.New("disk full mid-write")}, stubReader{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/search/replace",
		map[string]any{"query": "fmt"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "disk full mid-write")
}
