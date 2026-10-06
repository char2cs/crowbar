package promptorigin

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func Match(d *spec.Descriptor, prompt string) (spec.InjectedPromptSpec, bool) {
	if d == nil || len(d.InjectedPrompts) == 0 || prompt == "" {
		return spec.InjectedPromptSpec{}, false
	}
	body := strings.TrimLeft(prompt, " \t\r\n")
	if body == "" {
		return spec.InjectedPromptSpec{}, false
	}

	// A frame can arrive inside one wrapping tag, so the text under it is
	// matched too.
	inner := unwrap(body)

	var generic spec.InjectedPromptSpec
	var found bool
	for _, p := range d.InjectedPrompts {
		if p.Needle == "" || (!strings.HasPrefix(body, p.Needle) && !strings.HasPrefix(inner, p.Needle)) {
			continue
		}
		if p.Kind != "" {
			return p, true
		}
		if !found {
			generic = p
			found = true
		}
	}
	return generic, found
}

func Declared(d *spec.Descriptor) bool {
	return d != nil && len(d.InjectedPrompts) > 0
}

// unwrap drops one leading `<tag ...>` line, returning "" when there is none.
func unwrap(body string) string {
	if !strings.HasPrefix(body, "<") {
		return ""
	}
	end := strings.IndexByte(body, '>')
	if end < 0 || strings.ContainsAny(body[:end], "\r\n") {
		return ""
	}
	return strings.TrimLeft(body[end+1:], " \t\r\n")
}
