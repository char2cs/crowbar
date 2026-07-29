package agent

import "strings"

type TemplateCtx struct {
	// Tmp is the PER-SEGMENT directory for a provider's per-spawn config: it is
	// deleted when that segment's CLI exits. Nothing a provider must keep may live
	// here — a provider owns its own state (see codex.yaml: Crowbar does not own
	// codex's home, precisely so it can never delete codex's sessions).
	Tmp string
	ID  string
	// Context is the single document Crowbar injects into a spawning CLI: the
	// chat-title instruction, the handed-off conversation, or both, composed by
	// the agent usecase. One document (not one per concern) because a provider
	// may only have ONE such channel — codex delivers both through the same
	// `developer_instructions` key, so two independent injections would collide.
	Context string
	// LedgerDir is where the conversation already lives: one file per turn, named
	// <seq>-<timestamp>-<role>-<provider>.turn.
	LedgerDir string
	// LedgerCut names the last turn the provider being resumed has already seen, so
	// it knows where to START reading.
	LedgerCut string
	// ContextPointer is the SHORT message (config.yaml's handoff_pointer) that sends
	// a provider to LedgerDir. It exists because a resumed codex can only be reached
	// through a USER MESSAGE — and pasting the whole handed-off transcript into the
	// chat is noise the user has to scroll past. An agent reads files; point it at
	// the one that is already there.
	ContextPointer string
	Cwd            string
	CrowbarHook    string
	Segid          string
	// RunnerToken authenticates this runner's MCP calls back into the daemon. The
	// segment id alone cannot: the agent controls the process holding it and can
	// read its own argv, so an agent that learned a sibling's segment could
	// otherwise assume that sibling's scope. Minted per daemon boot; runners never
	// outlive a boot, so it needs no persistence and revokes itself.
	RunnerToken string
	Provider    string
	ProjectID   string
	RepoID      string
	WorkspaceID string
}

// ScopeFlags renders the project/repo/workspace scope as the CLI flag list every
// in-PTY crowbar callback (`hook`, `handoff dump`) needs to rebuild its
// workspace-nested agent API path.
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
//  2. Omit --repo entirely when the repo id is empty. Belt-and-braces for (1): an
//     absent flag parses as "", which is exactly what cmd/crowbar's scopedAgentPath
//     reads to select the project-home agent mount, whereas a dangling `--repo=`
//     leaves a token for anything downstream to misread. It mattered most while
//     Crowbar still asked the LLM to RETYPE a command line carrying these flags — a
//     dangling `--repo=` invited the model to "helpfully" fill it in — and that path
//     is retired, but the descriptors still render this into flat shell strings.
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
		"{context}", ctx.Context,
		"{ledger_dir}", ctx.LedgerDir,
		"{ledger_cut}", ctx.LedgerCut,
		"{context_pointer}", ctx.ContextPointer,
		"{cwd}", ctx.Cwd,
		"{crowbar_hook}", ctx.CrowbarHook,
		"{crowbar}", ctx.CrowbarHook, // same binary as {crowbar_hook}; friendlier for non-hook commands
		"{segid}", ctx.Segid,
		"{runner_token}", ctx.RunnerToken,
		"{provider}", ctx.Provider,
		"{project_id}", ctx.ProjectID,
		"{repo_id}", ctx.RepoID,
		"{workspace_id}", ctx.WorkspaceID,
	)
	return r.Replace(s)
}
