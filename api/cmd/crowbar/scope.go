package main

import "github.com/spf13/cobra"

// scopedAgentPath builds the workspace-nested agent API path (Task 3) for the
// given project/repo/workspace ids, appending suffix (which may carry its own
// path segments and query string) after the .../agent segment.
func scopedAgentPath(
	project, repo, workspace, suffix string,
) string {
	return "/v0/projects/" + project + "/repos/" + repo + "/workspaces/" + workspace + "/agent" + suffix
}

// bindScopeFlags registers the --project/--repo/--workspace flags shared by
// every in-PTY CLI callback (hook, chat rename, handoff dump) that must build
// a workspace-nested agent API path — Task 3 nested those routes under
// .../workspaces/:wsId/agent, so every callback now needs its scope passed in
// explicitly rather than assuming a flat /v0/agent/... URL.
func bindScopeFlags(
	cmd *cobra.Command,
	project, repo, workspace *string,
) {
	cmd.Flags().StringVar(project, "project", "", "project id")
	cmd.Flags().StringVar(repo, "repo", "", "repo id")
	cmd.Flags().StringVar(workspace, "workspace", "", "workspace id")
}
