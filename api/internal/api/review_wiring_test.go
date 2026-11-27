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

func windowedReviewRoutes() []string {
	const chat = "/v0/chats/:chatId"
	return []string{
		"GET " + chat + "/review/outline",
		"GET " + chat + "/review/patch",
		"GET " + chat + "/review/search",
	}
}

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
