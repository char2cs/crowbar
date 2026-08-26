package models

import "strings"

type TemplateCtx struct {
	Tmp string
	ID  string

	Message string

	Context string

	ContextPointer string

	ChatID string

	GapTurns string

	Model  string
	Effort string

	Cwd         string
	CrowbarHook string
	Segid       string

	RunnerToken string

	Provider    string
	ProjectID   string
	RepoID      string
	WorkspaceID string

	// Socket is the unix socket path an api-transport provider's `serve` and
	// `attach` argv template against ({socket} in codex.yaml's
	// runtime.api.serve/.attach). Short-lived, per-runner, and NEVER under a
	// Crowbar worktree — macOS's sun_path is a hard 104 bytes.
	Socket string

	// Session is the session/thread id an api-transport provider's live
	// connection has established ({session_id} in codex.yaml's
	// runtime.api.attach) — set only AFTER EstablishSession has run, so
	// attach's argv can point at the SAME conversation `prompt`'s turn/start
	// acts on rather than a disconnected one of its own.
	Session string
}

func (c TemplateCtx) ScopeFlags() string {
	flags := "--project=" + c.ProjectID + " --workspace=" + c.WorkspaceID
	if c.RepoID != "" {
		flags += " --repo=" + c.RepoID
	}
	return flags
}

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

		"{crowbar}", c.CrowbarHook,
		"{segid}", c.Segid,
		"{runner_token}", c.RunnerToken,
		"{provider}", c.Provider,
		"{project_id}", c.ProjectID,
		"{repo_id}", c.RepoID,
		"{workspace_id}", c.WorkspaceID,
		"{socket}", c.Socket,
		"{session_id}", c.Session,
	)
}
