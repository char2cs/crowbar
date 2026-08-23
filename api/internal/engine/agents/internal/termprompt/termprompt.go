package termprompt

import (
	"strings"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func Match(d *spec.Descriptor, screen string) (models.TerminalPrompt, bool) {
	if d == nil || len(d.TerminalPrompts) == 0 || screen == "" {
		return models.TerminalPrompt{}, false
	}
	haystack := squeeze(screen)
	if haystack == "" {
		return models.TerminalPrompt{}, false
	}

	var generic models.TerminalPrompt
	var found bool
	for _, p := range d.TerminalPrompts {
		needle := squeeze(p.Needle)
		if needle == "" || !strings.Contains(haystack, needle) {
			continue
		}
		if p.Kind != "" {
			return models.TerminalPrompt{Kind: p.Kind, Needle: p.Needle}, true
		}
		if !found {
			generic = models.TerminalPrompt{Needle: p.Needle}
			found = true
		}
	}
	return generic, found
}

func Declared(d *spec.Descriptor) bool {
	return d != nil && len(d.TerminalPrompts) > 0
}

func squeeze(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
