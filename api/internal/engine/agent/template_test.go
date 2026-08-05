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
	require.Equal(t, "developer_instructions=HI", agent.Expand("developer_instructions={context}", agent.TemplateCtx{Context: "HI"}))
	require.Equal(t,
		"/bin/crowbar hook turn_stop --segment seg1 --provider claude",
		agent.Expand("{crowbar_hook} hook turn_stop --segment {segid} --provider {provider}", ctx))
	// {crowbar} is the same binary as {crowbar_hook}, spelled for the callbacks that
	// are not hooks.
	require.Equal(t,
		"/bin/crowbar handoff dump chat-1",
		agent.Expand("{crowbar} handoff dump {id}",
			agent.TemplateCtx{CrowbarHook: "/bin/crowbar", ID: "chat-1"}))
}

// {runner_token} is what an in-PTY `crowbar mcp` relay authenticates with. It is a
// token of its own rather than a reuse of {segid} because the agent controls the
// process holding the segment id and can read its own argv: a segment id alone
// would let an agent that learned a sibling's id assume that sibling's scope.
func TestExpand_RunnerToken(t *testing.T) {
	got := agent.Expand("tok={runner_token}", agent.TemplateCtx{RunnerToken: "abc123"})
	require.Equal(t, "tok=abc123", got)
}

func TestExpand_ReplacesScopeTokens(t *testing.T) {
	ctx := agent.TemplateCtx{ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}
	require.Equal(t,
		"--project p1 --repo r1 --workspace w1",
		agent.Expand("--project {project_id} --repo {repo_id} --workspace {workspace_id}", ctx))
}

// {scope_flags} is the token the shipped descriptors use. Its contract is the one
// that keeps project-home callbacks alive: the `=` form (so an empty value can never
// swallow the following token when the flat hook command is word-split by the shell)
// and NO --repo at all when there is no repo id.
// cmd/crowbar/scope_roundtrip_test.go proves the end of that chain.
func TestExpand_ScopeFlagsOmitsRepoWhenEmpty(t *testing.T) {
	require.Equal(t,
		"--project=p1 --workspace=w1",
		agent.Expand("{scope_flags}", agent.TemplateCtx{ProjectID: "p1", WorkspaceID: "w1"}),
		"a project-home workspace has no repo id — the flag must be omitted, not left empty")

	require.Equal(t,
		"--project=p1 --workspace=w1 --repo=r1",
		agent.Expand("{scope_flags}", agent.TemplateCtx{ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}),
		"repo-home and worktree workspaces carry a repo id and must keep it")
}
