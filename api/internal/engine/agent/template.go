package agent

import "strings"

type TemplateCtx struct {
	Tmp         string
	UUID        string
	ID          string
	Handoff     string
	Cwd         string
	CwdSlug     string
	CrowbarHook string
	SessionID   string
}

func Expand(s string, ctx TemplateCtx) string {
	r := strings.NewReplacer(
		"{tmp}", ctx.Tmp,
		"{uuid}", ctx.UUID,
		"{id}", ctx.ID,
		"{handoff}", ctx.Handoff,
		"{cwd}", ctx.Cwd,
		"{cwd_slug}", ctx.CwdSlug,
		"{crowbar_hook}", ctx.CrowbarHook,
		"{session_id}", ctx.SessionID,
	)
	return r.Replace(s)
}
