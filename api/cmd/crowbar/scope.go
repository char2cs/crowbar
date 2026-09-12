package main

import (
	"os"

	"github.com/spf13/cobra"
)

// scopedAgentPath builds the agent API path for the given project/repo/workspace
// ids, appending suffix (which may carry its own path segments and query string)
// after the .../chats segment.
//
// A project-level HOME workspace has NO repo id: agentWorkspaceReader.WorktreeDir
// returns an empty RepoID for it (the project-level home "has no repo id to
// resolve a slug from"; see usecases/container.go AgentChatsDir). Its agent
// surface is mounted under the home group (/v0/projects/:projectId/home/chats/...)
// by home.Register — NOT under .../repos/:repoId/workspaces/:wsId — so with an
// empty repo we must emit the HOME path or the in-PTY callbacks (hook, chat
// rename, handoff dump) would 404 on /repos//workspaces/.../chats. Repo-home
// (Kind=git / IsDefault) and worktrees both carry a repo id and take the
// workspace-scoped branch below.
func scopedAgentPath(
	project, repo, workspace, suffix string,
) string {
	if repo == "" {
		return "/v0/projects/" + project + "/home/chats" + suffix
	}
	return "/v0/projects/" + project + "/repos/" + repo + "/workspaces/" + workspace + "/chats" + suffix
}

// bindScopeFlags registers the --project/--repo/--workspace flags shared by
// every in-PTY CLI callback (hook, chat rename, handoff dump) that must build
// a workspace-nested agent API path — Task 3 nested those routes under
// .../workspaces/:wsId/chats, so every callback now needs its scope passed in
// explicitly rather than assuming a flat /v0/chats/... URL.
//
// The caller side of this contract is engine/agent's {scope_flags} template
// token (TemplateCtx.ScopeFlags), which renders these as `--project=…` /
// `--workspace=…` and OMITS --repo entirely for a project-home workspace. That
// omission is what makes an empty --repo actually reach scopedAgentPath's home
// branch: a bare `--repo ` in the flat shell string of a hook command is dropped
// by the shell, after which pflag swallows the following token as its value.
// See scope_roundtrip_test.go, which drives the real shell → real cobra path.
func bindScopeFlags(
	cmd *cobra.Command,
	project, repo, workspace, home *string,
) {
	cmd.Flags().StringVar(project, "project", "", "project id")
	cmd.Flags().StringVar(repo, "repo", "", "repo id")
	cmd.Flags().StringVar(workspace, "workspace", "", "workspace id")
	cmd.Flags().StringVar(home, "home", "", "crowbar home this callback must operate against")
}

// applyHomeOverride sets CROWBAR_HOME for this process when home is non-empty.
// Every in-PTY callback (hook, mcp, handoff dump) is a fresh, short-lived
// process invoked once per event/session, so mutating process-global state
// here at startup is race-free — there is no concurrent second invocation in
// the same process to race against.
//
// Without this, a callback resolves its home from whatever CROWBAR_HOME the
// vendor CLI's own hook/subprocess mechanism forwards, which is not
// guaranteed, and silently falls back to the real ~/.crowbar when absent —
// the exact mechanism that let an isolated dev instance's hook events land in
// production. home is now baked into the command line at spawn time
// (TemplateCtx.CrowbarHome / ScopeFlags, see spawnplan.go), so this override
// always wins over ambient environment.
func applyHomeOverride(home string) {
	if home != "" {
		_ = os.Setenv("CROWBAR_HOME", home)
	}
}
