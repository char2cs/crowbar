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

	Cwd         string
	CrowbarHook string
	Segid       string

	RunnerToken string

	// HookBase is the daemon's loopback hook server (hookserver.Server.URL), empty
	// when none is published. {hook_url} is built from it.
	HookBase string

	Provider    string
	ProjectID   string
	RepoID      string
	WorkspaceID string

	// CrowbarHome is the crowbar home this spawn was resolved against
	// (spawnPaths.crowbarHome in the runner package). Every in-PTY callback
	// (hook, mcp, handoff) must operate against THIS home, not whatever
	// CROWBAR_HOME the vendor CLI's own hook/subprocess mechanism happens to
	// forward — which is not guaranteed, and silently defaults to the user's
	// real ~/.crowbar when absent. Baking it into the command line here,
	// exactly like project/repo/workspace already are, makes delivery correct
	// regardless of what environment the callback inherits.
	CrowbarHome string

	// DaemonSocket is the unix socket the daemon actually bound. Callbacks dial
	// it as handed over (--host) and never derive one: the daemon owns its socket,
	// and a path derived from --home can differ from the one it bound.
	DaemonSocket string

	// Session is the session id an api-transport provider's live
	// connection has established ({session_id} in a descriptor's
	// runtime.api.attach) — set only AFTER EstablishSession has run, so
	// attach's argv can point at the SAME conversation `prompt`'s turn call
	// acts on rather than a disconnected one of its own.
	Session string

	// PermissionVars is the current chat's permission level's own named
	// values, exactly as its descriptor's permission_levels.<level>.vars
	// declared them — opaque to Go. Referenced as {permission.<key>}, the
	// same dotted-family shape as suggestion_label.* (see vocabulary.yaml's
	// own permission.* entry), for a transport (a session-creating call) whose
	// spawn-time behavior is a request field, not an argv flag Apply's
	// pass_arg can reach.
	PermissionVars map[string]string
}

// HookURL is where this runner's hooks are posted by a CLI that posts them itself:
// the runner-keyed route on the hook server, under the project/repo scope the relay
// built its path from — a project home has no repo and takes the home branch. The
// descriptor appends the event name.
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
	if c.DaemonSocket != "" {
		flags += " --host=" + shellWord("unix://"+c.DaemonSocket)
	}
	return flags
}

// shellWord is s as one shell word. {crowbar_hook} and {scope_flags} are only
// ever rendered into hook commands the vendor CLI runs through a shell, where
// a path with a space would split. Single quotes also survive the JSON and
// TOML strings those commands are embedded in (a path holding a quote itself
// cannot be embedded in either).
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
		"{crowbar_hook}", shellWord(c.CrowbarHook),
		"{crowbar_home}", c.CrowbarHome,
		"{hook_url}", c.HookURL(),
		"{hook_curl}", c.HookCurl(),

		"{crowbar}", c.CrowbarHook,
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
