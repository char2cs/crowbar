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
		if !opens(p, body) && !opens(p, inner) {
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

// opens is whether text begins as p declares: by its needle, else its anchored match.
func opens(p spec.InjectedPromptSpec, text string) bool {
	if text == "" {
		return false
	}
	if p.Needle != "" {
		return strings.HasPrefix(text, p.Needle)
	}
	return p.Match.Matches(text)
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
