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

func Expand(s string, ctx TemplateCtx) string {
	r := strings.NewReplacer(
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
