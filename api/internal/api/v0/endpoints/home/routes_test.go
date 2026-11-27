package home_test

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentroutes "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home"
)

func TestHomeMountsEveryAgentRoute(t *testing.T) {
	wsRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		scope := r.Group("/scope")
		agentroutes.Register(scope, r.Group("/v0/chats/:chatId"), scope, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}, "/scope")

	homeRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		home.Register(
			r.Group("/scope"),
			nil, nil, nil, nil,
			nil, nil,
			nil, nil, nil, nil, nil,
			nil,
			nil, nil,
		)
	}, "/scope/home")

	require.NotEmpty(t, wsRoutes, "the workspace group must mount agent routes at all")
	for _, route := range wsRoutes {
		assert.Contains(t, homeRoutes, route,
			"the /home group is missing agent route %q that the workspace group mounts — a chat on a "+
				"project-home or repo-home workspace would 404 on it", route)
	}
	for _, gone := range []string{"POST /chats/hooks", "POST /chats/hooks/await", "POST /chats/hooks/abandon"} {
		assert.NotContains(t, wsRoutes, gone)
		assert.NotContains(t, homeRoutes, gone)
	}
	assert.Contains(t, homeRoutes, "POST /chats/runners/:segid/hooks/:provider/:event")
}

func TestRoutes_ServeNoTranscriptReads(t *testing.T) {
	wsRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		scope := r.Group("/scope")
		agentroutes.Register(scope, r.Group("/v0/chats/:chatId"), scope, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}, "/scope")
	homeRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		home.Register(r.Group("/scope"), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}, "/scope/home")

	for _, gone := range []string{
		"GET /chats/:id/messages", "GET /chats/:id/activity", "GET /chats/:id/activity/:toolId/payload",
		"GET /chats/:id/choices", "GET /chats/:id/pending-prompt",
	} {
		assert.NotContains(t, wsRoutes, gone)
		assert.NotContains(t, homeRoutes, gone)
	}
	assert.Contains(t, homeRoutes, "POST /chats/:id/choices/:choiceId/answer", "an answer is a command, not a read")
}

func agentSubRoutes(t *testing.T, mount func(*gin.Engine), prefix string) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mount(r)

	var out []string
	for _, ri := range r.Routes() {
		path, ok := strings.CutPrefix(ri.Path, prefix)
		if !ok || !strings.HasPrefix(path, "/chats") {
			continue
		}
		out = append(out, ri.Method+" "+path)
	}
	return out
}

func TestHomeTerminalsAreServedByTheOneTerminalHandlerSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	home.Register(
		r.Group("/scope"),
		nil, nil, nil, nil,
		nil, nil,
		nil, nil, nil, nil, nil,
		nil,
		nil, nil,
	)
	want := map[string]string{
		"GET /scope/home/terminals":               "(*Handlers).ListSessions",
		"POST /scope/home/terminals":              "(*Handlers).CreateSession",
		"DELETE /scope/home/terminals/:sessionId": "(*Handlers).KillSession",
		"GET /scope/home/terminals/:sessionId/ws": "(*Handlers).WS",
	}
	got := map[string]string{}
	for _, ri := range r.Routes() {
		got[ri.Method+" "+ri.Path] = ri.Handler
	}
	for route, fn := range want {
		h, ok := got[route]
		require.True(t, ok, "home must mount %s", route)
		assert.Contains(t, h, "endpoints/terminal/handlers.", "%s must be the terminal handler set's", route)
		assert.Contains(t, h, fn, "%s", route)
	}
}
