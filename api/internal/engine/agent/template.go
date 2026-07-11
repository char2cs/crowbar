package agent

import "strings"

type TemplateCtx struct {
	// Tmp is the PER-SEGMENT directory: it is deleted when that segment's CLI
	// exits, so nothing that must survive a provider switch may live here.
	Tmp string
	// ChatDir is the per-CHAT directory, alive for as long as the chat is. It is
	// where a provider keeps state it needs across segments — codex's CODEX_HOME,
	// whose rollouts `codex resume` reads back. Pointing that at Tmp meant codex's
	// own session was deleted the instant it was switched away from, so switching
	// BACK to codex resumed a thread that no longer existed and the CLI died on
	// startup ("no rollout found for thread id ...").
	ChatDir string
	ID      string
	// Context is the single document Crowbar injects into a spawning CLI: the
	// chat-title instruction, the handed-off conversation, or both, composed by
	// the agent usecase. One document (not one per concern) because a provider
	// may only have ONE such channel — codex delivers both through the same
	// `developer_instructions` key, so two independent injections would collide.
	Context     string
	Cwd         string
	CrowbarHook string
	Segid       string
	Provider    string
	Chatid      string
	ProjectID   string
	RepoID      string
	WorkspaceID string
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
		"{chat_dir}", ctx.ChatDir,
		"{id}", ctx.ID,
		"{context}", ctx.Context,
		"{cwd}", ctx.Cwd,
		"{crowbar_hook}", ctx.CrowbarHook,
		"{crowbar}", ctx.CrowbarHook, // same binary as {crowbar_hook}; friendlier for non-hook commands
		"{segid}", ctx.Segid,
		"{provider}", ctx.Provider,
		"{chatid}", ctx.Chatid,
		"{project_id}", ctx.ProjectID,
		"{repo_id}", ctx.RepoID,
		"{workspace_id}", ctx.WorkspaceID,
	)
	return r.Replace(s)
}
