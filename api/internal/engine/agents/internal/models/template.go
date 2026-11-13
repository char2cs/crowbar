package models

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

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

	Cwd        string
	CrowbarBin string
	Segid      string

	RunnerToken string

	// HookBase is the daemon's loopback hook server (hookserver.Server.URL), empty
	// when none is published. {hook_url} is built from it.
	HookBase string

	Provider    string
	ProjectID   string
	RepoID      string
	WorkspaceID string

	// CrowbarHome is the home this spawn resolved against. It is baked into callback
	// command lines because a CLI need not forward CROWBAR_HOME, and a missing one
	// silently falls back to the user's real ~/.crowbar.
	CrowbarHome string

	// AttachmentsDir is the chat's attachment store, where pasted files live.
	AttachmentsDir string

	// DaemonHost is the address the daemon bound (unix:// or tcp://). Callbacks
	// dial it as handed over (--host, {daemon_host}) and never derive one: a socket
	// derived from --home can differ from the bound one, and TCP has none.
	DaemonHost string

	// Session is the api connection's session id ({session_id}), set only after
	// EstablishSession so attach's argv joins the SAME conversation prompts act on.
	Session string

	// PermissionVars is the chat's permission_levels.<level>.vars, opaque to Go and
	// referenced as {permission.<key>}, for transports that set permissions in a
	// request field rather than an argv flag.
	PermissionVars map[string]string
	// ModelParts is Model cut into the descriptor's named parts, sent to an api
	// session call as model.<name>.
	ModelParts map[string]string
	// PermissionLevel is the chat's resolved level, which a forked view's argv
	// renders through permission_levels.<level>.apply.
	PermissionLevel string
}

// HookURL is where this runner's hooks are posted: the runner-keyed route on the
// hook server, under its project/repo scope — a project home has no repo and
// takes the home branch. The descriptor appends the event name.
func (c TemplateCtx) HookURL() string {
	if c.HookBase == "" {
		return ""
	}
	scope := "/v0/projects/" + c.ProjectID + "/home"
	if c.RepoID != "" {
		scope = "/v0/projects/" + c.ProjectID + "/repos/" + c.RepoID
	}
	return c.HookBase + scope + "/chats/runners/" + c.Segid + "/hooks/" + c.Provider
}

// HookCurl is the curl command a CLI that can only run a command (a status
// line, a hook entry) posts its stdin through. The token sits in the config
// file, not argv; --fail makes a refused post exit non-zero instead of passing.
func (c TemplateCtx) HookCurl() string {
	return "curl --fail --silent --show-error --config " + shellWord(filepath.Join(c.Tmp, "hook.curl"))
}

func (c TemplateCtx) ScopeFlags() string {
	flags := "--project=" + c.ProjectID + " --workspace=" + c.WorkspaceID
	if c.RepoID != "" {
		flags += " --repo=" + c.RepoID
	}
	if c.CrowbarHome != "" {
		flags += " --home=" + shellWord(c.CrowbarHome)
	}
	if c.DaemonHost != "" {
		flags += " --host=" + shellWord(c.DaemonHost)
	}
	return flags
}

// shellWord is s as one shell word. {scope_flags} and {hook_curl} are rendered
// into commands a vendor CLI runs through a shell, where a path with a space
// would split; single quotes also survive the JSON and TOML embedding them.
func shellWord(s string) string {
	if s == "" || strings.Trim(s, shellSafe) == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

const shellSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./:=@%+-"

func (c TemplateCtx) Replacer() *strings.Replacer {
	pairs := c.pairs()
	for k, v := range c.PermissionVars {
		pairs = append(pairs, "{permission."+k+"}", v)
	}
	return strings.NewReplacer(pairs...)
}

// TemplateVars names every {var} an argv template may reference, besides the
// {permission.<key>} family.
func TemplateVars() []string {
	pairs := TemplateCtx{}.pairs()
	out := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, strings.Trim(pairs[i], "{}"))
	}
	return out
}

func (c TemplateCtx) pairs() []string {
	return []string{
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
		"{cwd_json}", jsonString(c.Cwd),
		"{crowbar_home}", c.CrowbarHome,
		"{attachments_dir}", c.AttachmentsDir,
		"{daemon_host}", c.DaemonHost,
		"{hook_url}", c.HookURL(),
		"{hook_curl}", c.HookCurl(),

		"{crowbar}", c.CrowbarBin,
		"{segid}", c.Segid,
		"{runner_token}", c.RunnerToken,
		"{provider}", c.Provider,
		"{project_id}", c.ProjectID,
		"{repo_id}", c.RepoID,
		"{workspace_id}", c.WorkspaceID,
		"{session_id}", c.Session,
	}
}

// jsonString is s as a quoted JSON string — also a valid TOML basic string, so
// a path can be embedded in a `-c key=<toml>` value whatever characters it has.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
