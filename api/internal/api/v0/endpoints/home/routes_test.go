package home_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentroutes "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent"
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
// So: every /agent/... route the workspace group mounts must exist under /home
// too. A new agent route now fails HERE rather than in the app.
func TestHomeMountsEveryAgentRoute(t *testing.T) {
	wsRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		scope := r.Group("/scope")
		// settingsRG carries only the GLOBAL /settings/agent/providers write route,
		// which is deliberately NOT a home capability — agentSubRoutes filters it out
		// (it is not an /agent/... path), so home parity is unaffected by it.
		agentroutes.Register(scope, scope, nil, nil, nil, noopWS)
	}, "/scope")

	homeRoutes := agentSubRoutes(t, func(r *gin.Engine) {
		home.Register(
			r.Group("/scope"),
			nil, nil, nil, nil, nil, // home deps: unused, the routing table is what is under test
			noopWS,
			nil, nil, noopWS,
			nil,      // agent usecase
			nil, nil, // chat-folder usecase + broadcast
			noopWS, // agent WS
			func(rest, _ gin.HandlerFunc) gin.HandlerFunc { return rest },
		)
	}, "/scope/home")

	require.NotEmpty(t, wsRoutes, "the workspace group must mount agent routes at all")
	for _, route := range wsRoutes {
		assert.Contains(t, homeRoutes, route,
			"the /home group is missing agent route %q that the workspace group mounts — a chat on a "+
				"project-home or repo-home workspace would 404 on it", route)
	}
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
		if !ok || !strings.HasPrefix(path, "/agent/") {
			continue
		}
		out = append(out, ri.Method+" "+path)
	}
	return out
}

func noopWS(c *gin.Context) { c.Status(http.StatusOK) }
