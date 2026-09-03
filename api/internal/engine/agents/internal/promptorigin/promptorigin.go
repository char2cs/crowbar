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

	var generic spec.InjectedPromptSpec
	var found bool
	for _, p := range d.InjectedPrompts {
		if p.Needle == "" || !strings.HasPrefix(body, p.Needle) {
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
