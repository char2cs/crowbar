package handlers_test

import (
	"context"
	"errors"
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

// TestSearchHandlers_UnresolvedWorkspace_Returns404 proves Search 404s when no
// workspace was resolved onto the request, BEFORE the search engine is ever
// consulted — an unresolved chat must not reach the engine with an empty
// WorktreePath, which would search the daemon's own working directory.
func TestSearchHandlers_UnresolvedWorkspace_Returns404(
	t *testing.T,
) {
	eng := &recordingEngine{}
	r := newUnscopedRouterWith(eng)

	rec := do(r, http.MethodPost, "/v0/chats/ghost/search",
		map[string]any{"query": "fmt"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "workspace not found")
	assert.Empty(t, eng.seenPaths, "the engine must not be consulted without a workspace")
}

// TestReplaceHandlers_UnresolvedWorkspace_Returns404 is Search's proof mirrored
// onto Replace: an unresolved workspace must 404 before Replace is ever called.
func TestReplaceHandlers_UnresolvedWorkspace_Returns404(
	t *testing.T,
) {
	eng := &recordingEngine{}
	r := newUnscopedRouterWith(eng)

	rec := do(r, http.MethodPost, "/v0/chats/ghost/search/replace",
		map[string]any{"query": "fmt", "replacement": "log"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "workspace not found")
	assert.Empty(t, eng.seenPaths, "the engine must not be consulted without a workspace")
}

// TestSearchHandlers_EngineUnavailable_Returns503 proves Search refuses with
// 503, not a panic or a 500, when the daemon was wired up with no search
// engine at all — h.searchEng == nil is a real deployment state (the engine
// failed to construct), not a test-only impossibility.
func TestSearchHandlers_EngineUnavailable_Returns503(
	t *testing.T,
) {
	r := newRouterWith(nil)

	rec := do(r, http.MethodPost, "/v0/chats/chat1/search",
		map[string]any{"query": "fmt"})

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestReplaceHandlers_EngineUnavailable_Returns503 is the Replace-side
// counterpart of TestSearchHandlers_EngineUnavailable_Returns503.
func TestReplaceHandlers_EngineUnavailable_Returns503(
	t *testing.T,
) {
	r := newRouterWith(nil)

	rec := do(r, http.MethodPost, "/v0/chats/chat1/search/replace",
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
	r := newRouterWith(errSearchEngine{err: errors.New("ripgrep: exit status 2")})

	rec := do(r, http.MethodPost, "/v0/chats/chat1/search",
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
	r := newRouterWith(errSearchEngine{err: errors.New("disk full mid-write")})

	rec := do(r, http.MethodPost, "/v0/chats/chat1/search/replace",
		map[string]any{"query": "fmt"})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "disk full mid-write")
}
