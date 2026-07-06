package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestExpand_ReplacesKnownTokens(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: "/t", ID: "s9", Cwd: "/w", CrowbarHook: "/bin/crowbar", Segid: "seg1", Provider: "claude"}
	require.Equal(t, "/t/settings.json", agent.Expand("{tmp}/settings.json", ctx))
	require.Equal(t, "resume s9", agent.Expand("resume {id}", ctx))
	require.Equal(t, "handoff=HI", agent.Expand("handoff={handoff}", agent.TemplateCtx{Handoff: "HI"}))
	require.Equal(t,
		"/bin/crowbar hook turn_stop --segment seg1 --provider claude",
		agent.Expand("{crowbar_hook} hook turn_stop --segment {segid} --provider {provider}", ctx))
}
