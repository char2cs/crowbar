package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestExpand_ReplacesKnownTokens(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: "/t", UUID: "u1", ID: "s9", Cwd: "/w", CrowbarHook: "/bin/crowbar"}
	require.Equal(t, "--session-id u1", agent.Expand("--session-id {uuid}", ctx))
	require.Equal(t, "/t/settings.json", agent.Expand("{tmp}/settings.json", ctx))
	require.Equal(t, "resume s9", agent.Expand("resume {id}", ctx))
	require.Equal(t, "/bin/crowbar hook session_start", agent.Expand("{crowbar_hook} hook session_start", ctx))
}
