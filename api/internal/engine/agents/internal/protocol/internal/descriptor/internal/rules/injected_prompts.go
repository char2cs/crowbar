package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type injectedPrompts struct{}

func (injectedPrompts) Name() string { return "injected_prompts" }

func (injectedPrompts) Check(d *spec.Descriptor) error {
	for i, p := range d.InjectedPrompts {
		if err := checkOpening(d.ID, i, p); err != nil {
			return err
		}
		if p.TaskID != nil && len(p.Items) > 0 {
			return invalid(d.ID, "injected_prompts[%d]: name task_id or items, not both", i)
		}
		for j, item := range p.Items {
			if !item.Valid() {
				return invalid(d.ID, "injected_prompts[%d]: items[%d] must be a pattern with an id group", i, j)
			}
		}
		if p.Kind == "" {
			continue
		}
		if _, ok := spec.InjectedPromptKinds[p.Kind]; !ok {
			return invalid(d.ID, "injected_prompts[%d]: unknown kind %q", i, p.Kind)
		}
		if p.Kind == spec.InjectedPromptSubagentResult && p.ScopeID == nil {
			return invalid(d.ID, "injected_prompts[%d]: a subagent_result names its scope_id", i)
		}
	}
	return nil
}

// checkOpening holds an entry to one way of recognising its prompt, one that
// cannot match every user message.
func checkOpening(id string, i int, p spec.InjectedPromptSpec) error {
	if p.Match != nil && p.Needle != "" {
		return invalid(id, "injected_prompts[%d]: name needle or match, not both", i)
	}
	if p.Match != nil && !p.Match.Anchored() {
		return invalid(id, "injected_prompts[%d]: match must be a pattern anchored with ^", i)
	}
	if p.Match != nil {
		return nil
	}
	if strings.TrimSpace(p.Needle) == "" {
		return invalid(id, "injected_prompts[%d]: name a needle or a match, not empty or whitespace", i)
	}
	if p.Needle != strings.TrimLeft(p.Needle, " \t\r\n") {
		return invalid(id, "injected_prompts[%d]: needle must not start with whitespace", i)
	}
	return nil
}
