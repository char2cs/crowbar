// Package template expands descriptor placeholders against a spawn context.
//
// Expansion is a single pass of literal replacement. It never invokes a shell and
// never re-scans replacement text, so provider syntax inside a user's message
// stays literal data rather than becoming argv structure.
package template

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

// Expand replaces every supported placeholder in s. An unknown placeholder is
// left alone: descriptors are user-overridable, and silently blanking a token a
// descriptor author typo'd would hide the mistake behind an empty argument.
func Expand(s string, ctx models.TemplateCtx) string {
	return ctx.Replacer().Replace(s)
}
