package rules

import (
	"strings"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type terminalPrompts struct{}

func (terminalPrompts) Name() string { return "terminal_prompts" }

func (terminalPrompts) Check(d *spec.Descriptor) error {
	for i, p := range d.TerminalPrompts {
		if !hasContent(p.Needle) {
			return invalid(d.ID, "terminal_prompts[%d]: needle must contain letters or digits", i)
		}
		if p.Kind == "" {
			continue
		}
		if _, ok := spec.TerminalPromptKinds[p.Kind]; !ok {
			return invalid(d.ID, "terminal_prompts[%d]: unknown kind %q", i, p.Kind)
		}
	}
	return nil
}

func hasContent(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}
