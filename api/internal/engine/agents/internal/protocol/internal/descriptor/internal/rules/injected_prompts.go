package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type injectedPrompts struct{}

func (injectedPrompts) Name() string { return "injected_prompts" }

func (injectedPrompts) Check(d *spec.Descriptor) error {
	for i, p := range d.InjectedPrompts {
		if strings.TrimSpace(p.Needle) == "" {
			return invalid(d.ID, "injected_prompts[%d]: needle must not be empty or whitespace", i)
		}
		if p.Needle != strings.TrimLeft(p.Needle, " \t\r\n") {
			return invalid(d.ID, "injected_prompts[%d]: needle must not start with whitespace", i)
		}
		if p.Kind == "" {
			continue
		}
		if _, ok := spec.InjectedPromptKinds[p.Kind]; !ok {
			return invalid(d.ID, "injected_prompts[%d]: unknown kind %q", i, p.Kind)
		}
	}
	return nil
}
