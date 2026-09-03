package template

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

func Expand(s string, ctx models.TemplateCtx) string {
	return ctx.Replacer().Replace(s)
}
