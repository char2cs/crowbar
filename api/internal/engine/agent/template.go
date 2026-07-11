package agent

import "strings"

type TemplateCtx struct {
	Tmp          string
	ID           string
	Handoff      string
	Cwd          string
	CrowbarHook  string
	Segid        string
	Provider     string
	Chatid       string
	SystemPrompt string
	ProjectID    string
	RepoID       string
	WorkspaceID  string
}

// ScopeFlags renders the project/repo/workspace scope as the CLI flag list every
// in-PTY crowbar callback (`hook`, `chat rename`, `handoff dump`) needs to
// rebuild its workspace-nested agent API path.
//
// It exists because the naive `--project {project_id} --repo {repo_id} --workspace
// {workspace_id}` triple is BROKEN for project-home workspaces, and two properties
// fix it:
//
//  1. The `=` form, never a space-separated value. A rendered hook command is a
//     FLAT SHELL STRING (a descriptor's settings.json "command"), and Expand is a
//     plain text replacer with no quoting. A project-home workspace has NO repo id
//     (usecases WorktreeDir yields ""), so `--repo {repo_id}` renders as `--repo `
//     — the shell then collapses the empty token entirely, and pflag (which does
//     not reject a dash-prefixed value) eats the NEXT token as --repo's value.
//     `--repo --workspace WS` parses as repo="--workspace" plus a stray positional
//     WS, blowing the command's ExactArgs check and killing every callback. An
//     `--repo=` token can never swallow a neighbour.
//  2. Omit --repo entirely when the repo id is empty. Belt-and-braces for (1), and
//     necessary for config.yaml's title_instruction, whose command is RETYPED by
//     the LLM — a dangling `--repo=` invites the model to "helpfully" fill it in.
//     An absent flag parses as "", which is exactly what cmd/crowbar's
//     scopedAgentPath reads to select the project-home agent mount.
func (c TemplateCtx) ScopeFlags() string {
	flags := "--project=" + c.ProjectID + " --workspace=" + c.WorkspaceID
	if c.RepoID != "" {
		flags += " --repo=" + c.RepoID
	}
	return flags
}

func Expand(s string, ctx TemplateCtx) string {
	r := strings.NewReplacer(
		"{scope_flags}", ctx.ScopeFlags(),
		"{tmp}", ctx.Tmp,
		"{id}", ctx.ID,
		"{handoff}", ctx.Handoff,
		"{cwd}", ctx.Cwd,
		"{crowbar_hook}", ctx.CrowbarHook,
		"{crowbar}", ctx.CrowbarHook, // same binary as {crowbar_hook}; friendlier for non-hook commands
		"{segid}", ctx.Segid,
		"{provider}", ctx.Provider,
		"{chatid}", ctx.Chatid,
		"{system_prompt}", ctx.SystemPrompt,
		"{project_id}", ctx.ProjectID,
		"{repo_id}", ctx.RepoID,
		"{workspace_id}", ctx.WorkspaceID,
	)
	return r.Replace(s)
}
