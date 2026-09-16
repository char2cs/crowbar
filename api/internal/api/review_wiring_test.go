package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// windowedReviewRoutes is the three-route windowed diff API, spelled at the
// depth the production tree mounts it: under the flat /v0/chats/:chatId
// prefix (spec §7.1) — the old /workspaces/:wsId twin the endpoint package
// used to also register is gone (spec §8 step 6).
func windowedReviewRoutes() []string {
	const chat = "/v0/chats/:chatId"
	return []string{
		"GET " + chat + "/review/outline",
		"GET " + chat + "/review/patch",
		"GET " + chat + "/review/search",
	}
}

// TestAPI_New_WindowedReviewRoutesMountedUnderV0 reads the real router's route
// table, so the assertion is about REGISTRATION rather than about a request
// happening to 200. A handler that answers on a hand-built test router but
// mounts at the wrong depth in the real container fails silently, and only a
// run against the live daemon would ever discover it.
func TestAPI_New_WindowedReviewRoutesMountedUnderV0(
	t *testing.T,
) {
	router, ok := newRealAPI(t).Handler().(*gin.Engine)
	require.True(t, ok, "the api container serves a gin engine")

	mounted := map[string]struct{}{}
	for _, r := range router.Routes() {
		mounted[r.Method+" "+r.Path] = struct{}{}
	}

	for _, route := range windowedReviewRoutes() {
		assert.Contains(t, mounted, route)
	}
}

// TestAPI_New_WindowedReviewRoutesAnswerOnRealRouter walks the whole production
// chain — every global middleware plus the chat group's resolveChatWorktree
// guard — to each of the three routes.
//
// The empty test home has no such chat, so the guard answers 404 "chat not
// found". That is the point: gin's 404 for an UNMOUNTED path is the plain-text
// "404 page not found", while a mounted route rejected by the guard answers
// the v0 error envelope. Only the second proves the route exists and its
// middleware chain ran.
func TestAPI_New_WindowedReviewRoutesAnswerOnRealRouter(
	t *testing.T,
) {
	handler := newRealAPI(t).Handler()

	paths := []string{
		"/v0/chats/chat1/review/outline",
		"/v0/chats/chat1/review/patch?path=a.go",
		"/v0/chats/chat1/review/search?q=todo",
	}
	for _, path := range paths {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))

		require.Equalf(t, http.StatusNotFound, rec.Code, "path %q", path)
		var env struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		require.NoErrorf(t, json.Unmarshal(rec.Body.Bytes(), &env), "path %q served gin's unmounted 404", path)
		assert.False(t, env.Success)
		assert.Equalf(t, "chat not found", env.Error, "path %q", path)
	}
}
