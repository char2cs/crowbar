package main

import "github.com/spf13/cobra"

// scopedAgentPath builds the agent API path for the given project/repo ids,
// appending suffix (which may carry its own path segments and query string)
// after the .../chats segment. workspace is accepted, but no longer part of
// the path (Task 17: the chat routes moved off .../workspaces/:wsId onto
// .../repos/:repoId, since a chat's workspace is optional and mutable) — kept
// as a parameter so bindScopeFlags's three callers need no signature change.
//
// A project-level HOME workspace has NO repo id: agentWorkspaceReader.WorktreeDir
// returns an empty RepoID for it (the project-level home "has no repo id to
// resolve a slug from"; see usecases/container.go AgentChatsDir). Its agent
// surface is mounted under the home group (/v0/projects/:projectId/home/chats/...)
// by home.Register — NOT under .../repos/:repoId — so with an
// empty repo we must emit the HOME path or the in-PTY callbacks (hook, chat
// rename, handoff dump) would 404 on /repos//chats. Repo-home
// (Kind=git / IsDefault) and worktrees both carry a repo id and take the
// repo-scoped branch below.
func scopedAgentPath(
	project, repo, workspace, suffix string,
) string {
	if repo == "" {
		return "/v0/projects/" + project + "/home/chats" + suffix
	}
	return "/v0/projects/" + project + "/repos/" + repo + "/chats" + suffix
}

// bindScopeFlags registers the --project/--repo/--workspace flags shared by
// every in-PTY CLI callback (hook, chat rename, handoff dump) that must build
// a scoped agent API path. --workspace is no longer read into that path
// (Task 17), but the flag stays: engine/agent's {scope_flags} template token
// still renders it, and dropping the flag itself would break parsing a
// command line that still carries it.
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
	project, repo, workspace *string,
) {
	cmd.Flags().StringVar(project, "project", "", "project id")
	cmd.Flags().StringVar(repo, "repo", "", "repo id")
	cmd.Flags().StringVar(workspace, "workspace", "", "workspace id")
}
