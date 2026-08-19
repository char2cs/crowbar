package models

import "strings"

// TemplateCtx is everything a descriptor may interpolate into an argv element,
// an env value or an injected config file.
type TemplateCtx struct {
	// Tmp is the PER-SPAWN directory for a provider's config: it is deleted when
	// that CLI exits. Nothing a provider must keep may live here — a provider owns
	// its own state, precisely so Crowbar can never delete its sessions.
	Tmp string
	ID  string

	// Message is a completed composer prompt. Descriptors place it in one argv
	// element with {message}; Expand never invokes a shell and never re-scans
	// replacement text, so provider syntax in the message stays literal.
	Message string

	// Context is the single document Crowbar injects into a spawning CLI: the
	// capability preamble, the handed-off conversation, or both. ONE document,
	// not one per concern, because a provider may only have one such channel —
	// codex delivers both through the same developer_instructions key, so two
	// independent injections would collide.
	Context string

	// ContextPointer is the SHORT message that sends a provider to the
	// conversation record instead of pasting it. It exists because a resumed
	// codex can only be reached through a USER MESSAGE, and pasting a whole
	// transcript into the chat is noise the user has to scroll past.
	ContextPointer string

	// ChatID is the Crowbar chat a callback or a pointer message refers to. It is
	// how a resumed CLI is told WHICH conversation to read back, now that the
	// record is a queryable store rather than a directory of files.
	ChatID string

	// GapTurns is how many turns were recorded while this provider was away. It
	// is what turns "read the log" into "read the last N turns": without it a
	// resumed CLI re-reads a conversation it has already been handed, which is
	// the wall of text the pointer exists to avoid.
	GapTurns string

	// Model and Effort are the chat's declared selection, interpolated by the
	// descriptor's own model:/effort: apply steps. They are EMPTY on a chat that
	// has chosen nothing, and the steps that read them are not rendered at all in
	// that case — so an empty field can never reach the argv as an empty flag
	// value (see selection.Steps).
	Model  string
	Effort string

	Cwd         string
	CrowbarHook string
	Segid       string

	// RunnerToken binds this runner's callbacks to the runner they claim to come
	// from. The segment id cannot: it is published on the chats API, so a call
	// naming one proves nothing. What the token buys is that acting as a sibling
	// cannot happen by ACCIDENT, and that a relay carrying these bytes has
	// nothing to authorise itself with. It is not containment against an agent
	// with a shell. Minted per daemon boot; runners never outlive a boot, so it
	// needs no persistence and revokes itself.
	RunnerToken string

	Provider    string
	ProjectID   string
	RepoID      string
	WorkspaceID string
}

// ScopeFlags renders the project/repo/workspace scope as the CLI flag list every
// in-PTY crowbar callback needs to rebuild its workspace-nested API path.
//
// The naive `--repo {repo_id}` triple is BROKEN for project-home workspaces, and
// two properties fix it:
//
//  1. The `=` form, never a space-separated value. A rendered hook command is a
//     FLAT SHELL STRING and Expand does no quoting. A project-home workspace has
//     NO repo id, so `--repo {repo_id}` renders as `--repo ` — the shell
//     collapses the empty token and pflag eats the NEXT token as the value.
//     `--repo --workspace WS` then parses as repo="--workspace" plus a stray
//     positional, blowing the command's arg check and killing every callback. An
//     `--repo=` token can never swallow a neighbour.
//  2. Omit --repo entirely when the repo id is empty. An absent flag parses as
//     "", which is exactly what selects the project-home mount, whereas a
//     dangling `--repo=` leaves a token for something downstream to misread.
func (c TemplateCtx) ScopeFlags() string {
	flags := "--project=" + c.ProjectID + " --workspace=" + c.WorkspaceID
	if c.RepoID != "" {
		flags += " --repo=" + c.RepoID
	}
	return flags
}

// Replacer returns the expansion table for this context. It lives here rather
// than in the template package so the set of supported placeholders is defined
// beside the fields they read.
func (c TemplateCtx) Replacer() *strings.Replacer {
	return strings.NewReplacer(
		"{scope_flags}", c.ScopeFlags(),
		"{tmp}", c.Tmp,
		"{id}", c.ID,
		"{message}", c.Message,
		"{context}", c.Context,
		"{context_pointer}", c.ContextPointer,
		"{chat_id}", c.ChatID,
		"{gap_turns}", c.GapTurns,
		"{model}", c.Model,
		"{effort}", c.Effort,
		"{cwd}", c.Cwd,
		"{crowbar_hook}", c.CrowbarHook,
		// same binary as {crowbar_hook}; friendlier for non-hook commands
		"{crowbar}", c.CrowbarHook,
		"{segid}", c.Segid,
		"{runner_token}", c.RunnerToken,
		"{provider}", c.Provider,
		"{project_id}", c.ProjectID,
		"{repo_id}", c.RepoID,
		"{workspace_id}", c.WorkspaceID,
	)
}
