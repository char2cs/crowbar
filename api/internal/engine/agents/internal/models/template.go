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

	RelayID    string
	RelayDir   string
	RelayToken string

	HookBase string

	Provider    string
	ProjectID   string
	RepoID      string
	WorkspaceID string

	CrowbarHome string

	AttachmentsDir string

	DaemonHost string

	Session string

	PermissionVars  map[string]string
	ModelParts      map[string]string
	PermissionLevel string
}

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

func (c TemplateCtx) HookCurl() string {
	return "curl --fail --silent --show-error --config " + shellWord(filepath.Join(c.Tmp, "hook.curl"))
}

func (c TemplateCtx) RelayTokenFile() string {
	if c.RelayID == "" {
		return ""
	}
	return filepath.Join(c.RelayDir, c.RelayID+".token")
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
		"{relay_id}", c.RelayID,
		"{relay_token_file}", c.RelayTokenFile(),
		"{relay_token}", c.RelayToken,
		"{provider}", c.Provider,
		"{project_id}", c.ProjectID,
		"{repo_id}", c.RepoID,
		"{workspace_id}", c.WorkspaceID,
		"{session_id}", c.Session,
	}
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
