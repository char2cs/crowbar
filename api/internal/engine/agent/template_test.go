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
	require.Equal(t,
		"/bin/crowbar chat rename c-9 \"x\"",
		agent.Expand("{crowbar} chat rename {chatid} \"x\"",
			agent.TemplateCtx{CrowbarHook: "/bin/crowbar", Chatid: "c-9"}))
}

func TestExpand_ReplacesScopeTokens(t *testing.T) {
	ctx := agent.TemplateCtx{ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}
	require.Equal(t,
		"--project p1 --repo r1 --workspace w1",
		agent.Expand("--project {project_id} --repo {repo_id} --workspace {workspace_id}", ctx))
}
