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

// TestHomeMountsEveryAgentRoute is a PARITY guard, and it exists because the
// absence of one cost a live bug.
//
// Chats are a home capability (00 agentic-engine spec), so the agent surface is
// mounted twice: once on the workspace group by agent.Register, and once on the
// /home group here — separately, because every home route must also carry
// RequireHomeWorkspace. That duplication is a standing invitation to drift, and
// it drifted: POST .../chats/:id/resume was added to agent.Register only, so
// reviving a dead chat 404'd on exactly the workspace kind where the user's chats
// live (project home). Unit tests passed; only the running app showed it.
//
// So: every /chats/... route the workspace group mounts must exist under /home
// too. A new agent route now fails HERE rather than in the app.
func TestHomeMountsEveryAgentRoute(t *testing.T) {
	wsRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		scope := r.Group("/scope")
		// settingsRG carries only the GLOBAL /settings/chat/providers write route,
		// which is deliberately NOT a home capability — agentSubRoutes filters it out
		// (it is not a /chats path), so home parity is unaffected by it.
		agentroutes.Register(scope, scope, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	}, "/scope")

	homeRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		home.Register(
			r.Group("/scope"),
			nil, nil, nil, nil, // home deps: unused, the routing table is what is under test
			nil, nil, // thread store + publisher
			nil, nil, nil, nil, nil, // the five agent concerns
			nil,      // chat-folder usecase
			nil, nil, // worktrees + nodes
		)
	}, "/scope/home")

	require.NotEmpty(t, wsRoutes, "the workspace group must mount agent routes at all")
	for _, route := range wsRoutes {
		assert.Contains(t, homeRoutes, route,
			"the /home group is missing agent route %q that the workspace group mounts — a chat on a "+
				"project-home or repo-home workspace would 404 on it", route)
	}
	// The relay transport is retired on both mounts: hooks have one ingress.
	for _, gone := range []string{"POST /chats/hooks", "POST /chats/hooks/await", "POST /chats/hooks/abandon"} {
		assert.NotContains(t, wsRoutes, gone)
		assert.NotContains(t, homeRoutes, gone)
	}
	assert.Contains(t, homeRoutes, "POST /chats/runners/:segid/hooks/:provider/:event")
}

// The transcript reads only the old client spoke are gone from both mounts:
// the ledger is read through the chat's feed and its blobs.
func TestRoutes_ServeNoTranscriptReads(t *testing.T) {
	wsRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		scope := r.Group("/scope")
		agentroutes.Register(scope, scope, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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

// agentSubRoutes registers routes via `mount` and returns every "/agent/..." route
// it produced, as "METHOD path" with the group prefix stripped, so the two mounts
// are comparable.
func agentSubRoutes(t *testing.T, mount func(*gin.Engine), prefix string) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mount(r)

	var out []string
	for _, ri := range r.Routes() {
		path, ok := strings.CutPrefix(ri.Path, prefix)
		// The whole chat surface lives under /chats — including folders, hooks
		// and providers — so it stays identifiable by one prefix now that the
		// /agent segment is gone. That identifiability is what makes this parity
		// guard possible at all.
		if !ok || !strings.HasPrefix(path, "/chats") {
			continue
		}
		out = append(out, ri.Method+" "+path)
	}
	return out
}

// TestHomeTerminalsAreServedByTheOneTerminalHandlerSet pins §7-B "one transport": the
// home group's terminal routes are the chat-scoped terminal handlers themselves (same
// list DTO, same PTY WebSocket), not a second copy that drifts — the old copy answered
// the list with a bare []string the client had to special-case.
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
