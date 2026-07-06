package agent

import "strings"

type TemplateCtx struct {
	Tmp         string
	ID          string
	Handoff     string
	Cwd         string
	CrowbarHook string
	Segid       string
	Provider    string
}

func Expand(s string, ctx TemplateCtx) string {
	r := strings.NewReplacer(
		"{tmp}", ctx.Tmp,
		"{id}", ctx.ID,
		"{handoff}", ctx.Handoff,
		"{cwd}", ctx.Cwd,
		"{crowbar_hook}", ctx.CrowbarHook,
		"{segid}", ctx.Segid,
		"{provider}", ctx.Provider,
	)
	return r.Replace(s)
}
