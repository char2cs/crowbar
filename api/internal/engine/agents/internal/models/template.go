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

	// PermissionVars is the current chat's permission level's own named
	// values, exactly as its descriptor's permission_levels.<level>.vars
	// declared them — opaque to Go. Referenced as {permission.<key>}, the
	// same dotted-family shape as suggestion_label.* (see vocabulary.yaml's
	// own permission.* entry), for a transport (codex's thread/start) whose
	// spawn-time behavior is a request field, not an argv flag Apply's
	// pass_arg can reach.
	PermissionVars map[string]string
}

func (c TemplateCtx) ScopeFlags() string {
	flags := "--project=" + c.ProjectID + " --workspace=" + c.WorkspaceID
	if c.RepoID != "" {
		flags += " --repo=" + c.RepoID
	}
	return flags
}

func (c TemplateCtx) Replacer() *strings.Replacer {
	pairs := []string{
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
	}
	for k, v := range c.PermissionVars {
		pairs = append(pairs, "{permission."+k+"}", v)
	}
	return strings.NewReplacer(pairs...)
}
